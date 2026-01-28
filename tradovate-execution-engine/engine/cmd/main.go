package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
	"tradovate-execution-engine/engine/UI"
	"tradovate-execution-engine/engine/config"
	"tradovate-execution-engine/engine/internal/auth"
	"tradovate-execution-engine/engine/internal/execution"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/marketdata"
	"tradovate-execution-engine/engine/internal/portfolio"
	"tradovate-execution-engine/engine/internal/tradovate"
	_ "tradovate-execution-engine/engine/strategies"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	symbol     = "MESH6"
	configFile = "config/config.json"
)

var (
	totalTests  int
	failedTests int
)

func main() {

	// tests.RunAllTests()

	// return

	p := tea.NewProgram(UI.InitialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}

	return
	//Pass to UI:

	// return

	//
	//	PROGRAM CONTROL FLOW - USE THIS FOR WHEN
	//
	mainLog := logger.NewLogger(500)
	orderLog := logger.NewLogger(500)

	// Ensure config directory exists
	_ = os.MkdirAll("../../config", 0755)

	// Initialize Config
	config, err := config.LoadOrCreateConfig(mainLog)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}

	//fmt.Println(config)

	// Get the global token manager
	tm := auth.NewTokenManager(config)

	// // Authenticate
	if err := tm.Authenticate(); err != nil {
		fmt.Println("Authentication error:", err)
		return
	}
	fmt.Println("Authentication Successful")

	return
	// Initialize OrderManager
	orderManager := execution.NewOrderManager(tm, config, orderLog)

	var marketDataClient *tradovate.TradovateWebSocketClient
	var tradingClient *tradovate.TradovateWebSocketClient
	var marketDataSubscriptionManager *tradovate.DataSubscriber
	var tradingClientSubscriptionManager *tradovate.DataSubscriber

	// Function to initialize/reinitialize clients with fresh tokens
	initializeClients := func() error {
		//Get fresh tokens
		accessToken, err := tm.GetAccessToken()
		if err != nil {
			return fmt.Errorf("failed to get access token: %w", err)
		}
		fmt.Println("\nAuth token aquired")

		tm.SetLogger(mainLog)

		mdToken, err := tm.GetMDAccessToken()
		if err != nil {
			return fmt.Errorf("failed to get MD token: %w", err)
		}
		fmt.Println("\nMarket data token aquired")

		// Create new WebSocket clients
		marketDataClient = tradovate.NewTradovateWebSocketClient(mdToken, config.Tradovate.Environment, "md")
		marketDataClient.SetLogger(mainLog)

		tradingClient = tradovate.NewTradovateWebSocketClient(accessToken, config.Tradovate.Environment, "")
		tradingClient.SetLogger(mainLog)

		// Create subscription managers
		marketDataSubscriptionManager = tradovate.NewDataSubscriptionManager(marketDataClient)
		marketDataSubscriptionManager.SetLogger(mainLog)

		tradingClientSubscriptionManager = tradovate.NewDataSubscriptionManager(tradingClient)
		tradingClientSubscriptionManager.SetLogger(mainLog)

		// Connect the clients
		if err := marketDataSubscriptionManager.Connect(); err != nil {
			fmt.Printf("\nerror connecting market data client: %w\n", err)
		}
		mainLog.Info("Market data WebSocket connected")

		if err := tradingClientSubscriptionManager.Connect(); err != nil {
			fmt.Printf("\nerror connecting trading client: %w\n", err)
		}
		fmt.Println("Trading WebSocket connected")

		// Set message handlers
		marketDataClient.SetMessageHandler(marketDataSubscriptionManager.HandleEvent)
		tradingClient.SetMessageHandler(tradingClientSubscriptionManager.HandleEvent)

		fmt.Println("Message Handlers Set")
		return nil
	}

	// Initialize clients for the first time
	if err := initializeClients(); err != nil {
		fmt.Printf("Failed to initialize clients: %v\n", err)
		return
	}

	//Set up order status handlers
	setupOrderHandlers := func() {
		tradingClientSubscriptionManager.OnOrderUpdate = func(data json.RawMessage) {
			var order struct {
				ID           int     `json:"id"`
				OrderType    string  `json:"orderType"`
				Action       string  `json:"action"`
				OrdStatus    string  `json:"ordStatus"` // Note: Tradovate uses "ordStatus" not "orderStatus"
				FilledQty    int     `json:"filledQty"`
				AvgFillPrice float64 `json:"avgFillPrice"`
				Timestamp    string  `json:"timestamp"`
				RejectReason string  `json:"rejectReason,omitempty"`
			}

			if err := json.Unmarshal(data, &order); err != nil {
				mainLog.Warnf("Failed to parse order update: %v", err)
				return
			}

			// Log all order updates for debugging
			//mainLog.Debugf("ORDER UPDATE: %+v", order)
			fmt.Printf("\nORDER UPDATE: %+v", order)

			// Handle different order statuses
			switch order.OrdStatus {
			case "PendingNew":
				mainLog.Infof("⏳ ORDER PENDING: ID=%d | %s %s",
					order.ID, order.Action, order.OrderType)

			case "Filled":
				mainLog.Infof("✅ ORDER FILLED: ID=%d | %s %s | Qty=%d | Avg Price=%.2f",
					order.ID, order.Action, order.OrderType, order.FilledQty, order.AvgFillPrice)
				orderLog.Infof("FILL | Order ID: %d | %s %d @ %.2f",
					order.ID, order.Action, order.FilledQty, order.AvgFillPrice)

			case "Rejected":
				mainLog.Errorf("❌ ORDER REJECTED: ID=%d | %s %s | Reason: %s",
					order.ID, order.Action, order.OrderType, order.RejectReason)
				orderLog.Errorf("REJECT | Order ID: %d | Reason: %s", order.ID, order.RejectReason)

			case "Working":
				mainLog.Infof("📋 ORDER WORKING: ID=%d | %s %s",
					order.ID, order.Action, order.OrderType)

			case "Canceled":
				mainLog.Infof("🚫 ORDER CANCELED: ID=%d", order.ID)

			case "Suspended":
				mainLog.Debugf("⏸️  ORDER SUSPENDED: ID=%d (part of order strategy)", order.ID)
			default:
				fmt.Printf("\n Unknown ORDER Status ?: ID=%d | %s %s\n",
					order.ID, order.Action, order.OrderType)
			}

		}
	}

	// Set up handlers initially
	setupOrderHandlers()
	fmt.Println("OnOrderUpdate Set")

	userID := tm.GetUserID()
	tracker := portfolio.NewPortfolioTracker(tradingClientSubscriptionManager, marketDataSubscriptionManager, userID, mainLog)

	if err := tracker.Start(config.Tradovate.Environment); err != nil {

		fmt.Printf("\nFailed to start PortfolioTracker: %v\n", err)
		return
	}

	fmt.Println("PortFolioTracker Started")

	//Start token refresh monitor with reconnection callback
	tm.StartTokenRefreshMonitor(func() {
		fmt.Println("\nReconnection complete after token refresh")
		//mainLog.Info("✅ Reconnection complete after token refresh")
	})

	fmt.Println("🚀 System initialized - monitoring for positions and token expiration")

	prevtoken, err := tm.GetAccessToken()
	fmt.Println("Access token:", prevtoken)
	fmt.Println("tdsubs: ", tradingClientSubscriptionManager.GetActiveSubscriptions())
	fmt.Println("mdsubs: ", marketDataSubscriptionManager.GetActiveSubscriptions())

	time.Sleep(1 * time.Minute)
	tm.RenewAccessToken()

	currenttoken, err := tm.GetAccessToken()
	fmt.Println("Token test", currenttoken == prevtoken)
	fmt.Println("tdsubs: ", tradingClientSubscriptionManager.GetActiveSubscriptions())
	fmt.Println("mdsubs: ", marketDataSubscriptionManager.GetActiveSubscriptions())

	select {}
	return

	var availableStrats []string
	availableStrats = execution.GetAvailableStrategies()
	fmt.Printf("Discovered %d registered strategies: %v\n", len(availableStrats), availableStrats)

	strat, err := execution.CreateStrategy("ma_crossover", mainLog)

	fmt.Println("Strategy Created ma_crossover")

	strat.SetParam("fast_length", "5")
	strat.SetParam("slow_length", "15")

	if err := strat.Init(orderManager); err != nil {
		fmt.Println("Failed to initialize strategy: " + err.Error())
		return
	}

	fmt.Println("Strategy Initialized")

	params := strat.GetParams()

	fmt.Println(params)

	var symbol string
	for _, param := range params {
		if param.Name == "symbol" {
			symbol = param.Value // Get the VALUE which is "MESH6"
			break
		}
	}

	// Track last bar to avoid exact duplicates
	type LastBar struct {
		Timestamp string
		Close     float64
	}
	var lastBar LastBar

	var historicalLoaded bool
	marketDataSubscriptionManager.AddChartHandler(func(update marketdata.ChartUpdate) {
		fmt.Printf("\n🔔 CHART UPDATE RECEIVED at %s\n", time.Now().Format("15:04:05"))
		fmt.Printf("📊 Chart handler called with %d charts\n", len(update.Charts))

		for _, chart := range update.Charts {
			fmt.Printf("Chart ID: %d | Bars: %d | Ticks: %d | EOH: %v\n",
				chart.ID, len(chart.Bars), len(chart.Ticks), chart.EOH)

			// Check for end of history marker
			if chart.EOH {
				fmt.Println("✅ End of historical data - now receiving live updates")
				// fmt.Printf("📊 SMAs initialized - Fast: %.2f | Slow: %.2f\n",
				// 	strat.GetMetrics()["Fast SMA"],
				// 	strat.GetMetrics()["Slow SMA"])

				// PRINT INITIAL SMA VALUES

				// Enable strategy for live trading
				if s, ok := strat.(interface{ SetEnabled(bool) }); ok {
					s.SetEnabled(true)
					fmt.Println("🚀 Strategy enabled for LIVE trading")
					fmt.Println("tdsubs: ", tradingClientSubscriptionManager.GetActiveSubscriptions())
					fmt.Println("mdsubs: ", marketDataSubscriptionManager.GetActiveSubscriptions())
				}

				historicalLoaded = true
				continue
			}

			fmt.Printf("\n=== Chart ID: %d ===\n", chart.ID)
			fmt.Printf("Number of bars: %d\n", len(chart.Bars))

			if !historicalLoaded {
				// Process each bar
				for _, bar := range chart.Bars {
					// Skip only exact duplicates (same timestamp AND same close price)
					if bar.Timestamp == lastBar.Timestamp && bar.Close == lastBar.Close {
						//fmt.Printf("  ⏭️  Skipping duplicate bar at %s\n", bar.Timestamp)
						continue
					}
					lastBar = LastBar{Timestamp: bar.Timestamp, Close: bar.Close}

					// Update SMAs with bar close price
					// fastValue := fastSMA.Update(bar.Close)
					// slowValue := slowSMA.Update(bar.Close)

					// Update Strategy
					if s, ok := strat.(interface {
						OnBar(string, float64) error
					}); ok {
						s.OnBar(bar.Timestamp, bar.Close)
					}

					//Parse and format time for display
					// t, err := time.Parse("2006-01-02T15:04Z", bar.Timestamp)
					// humanTime := bar.Timestamp
					// if err == nil {
					// 	humanTime = t.Local().Format("Jan 02 3:04 PM")
					// }

					// // Print bar with SMA values
					// fmt.Printf("Bar %d: %s | C=%.2f | Fast SMA(5)=%.2f | Slow SMA(15)=%.2f\n",
					// 	i+1,
					// 	humanTime,
					// 	bar.Close,
					// 	strat.GetMetrics()["Fast SMA"],
					// 	strat.GetMetrics()["Slow SMA"])
				}

				// PRINT LIVE SMA VALUES

				fmt.Printf("📊 Current Fast SMA: %.2f | Slow SMA: %.2f\n",
					strat.GetMetrics()["Fast SMA"],
					strat.GetMetrics()["Slow SMA"])
			}
		}
	})

	fmt.Println("Chart Handler Added")

	// Bar aggregator - collects quotes into minute bars
	type BarAggregator struct {
		currentMinute string
		open          float64
		high          float64
		low           float64
		close         float64
		firstTick     bool
	}

	var barAgg = &BarAggregator{firstTick: true}

	livebarcounter := 0

	marketDataSubscriptionManager.AddQuoteHandler(func(quote marketdata.Quote) {

		if !historicalLoaded {
			return
		}

		if trade, ok := quote.Entries["Trade"]; ok {
			price := trade.Price

			// Use the quote's actual timestamp, not time.Now()
			quoteTime, err := time.Parse(time.RFC3339, quote.Timestamp)
			if err != nil {
				return
			}

			currentMinute := quoteTime.UTC().Truncate(time.Minute).Format("2006-01-02T15:04Z")

			// New minute = new bar
			if currentMinute != barAgg.currentMinute {
				// Close previous bar if exists
				if !barAgg.firstTick {
					// Parse and format the timestamp
					//t, _ := time.Parse("2006-01-02T15:04Z", barAgg.currentMinute)
					//humanTime := t.Local().Format("Jan 02 3:04 PM")

					// fmt.Printf("\n📊 BAR CLOSE: %s | O:%.2f H:%.2f L:%.2f C:%.2f\n",
					// 	humanTime, barAgg.open, barAgg.high, barAgg.low, barAgg.close)
					// Update SMAs with bar close
					//fastValue := fastSMA.Update(barAgg.close)
					//slowValue := slowSMA.Update(barAgg.close)

					// Update Strategy
					if s, ok := strat.(interface {
						OnBar(string, float64) error
					}); ok {
						s.OnBar(barAgg.currentMinute, barAgg.close)
					}
					livebarcounter++
					fmt.Println("\nLiveBarCount: ", livebarcounter)
					//fmt.Printf("   Fast SMA: %.2f | Slow SMA: %.2f\n", fastValue, slowValue)

					fmt.Printf("   Fast SMA: %.2f | Slow SMA: %.2f\n",
						strat.GetMetrics()["Fast SMA"],
						strat.GetMetrics()["Slow SMA"])
				}

				// Start new bar
				barAgg.currentMinute = currentMinute
				barAgg.open = price
				barAgg.high = price
				barAgg.low = price
				barAgg.close = price
				barAgg.firstTick = false
			} else {
				// Update current bar
				if price > barAgg.high {
					barAgg.high = price
				}
				if price < barAgg.low {
					barAgg.low = price
				}
				barAgg.close = price
			}
		}
	})

	fmt.Println("Quote Handler added")

	go func() {
		//marketDataSubscriptionManager.Connect()
		marketDataSubscriptionManager.SubscribeQuote(symbol)

		mdparams := marketdata.HistoricalDataParams{
			Symbol: symbol,
			ChartDescription: marketdata.ChartDesc{
				UnderlyingType:  "MinuteBar",
				ElementSize:     1,
				ElementSizeUnit: "UnderlyingUnits",
			},
			TimeRange: marketdata.TimeRange{
				ClosestTimestamp: time.Now().Format(time.RFC3339),
				AsMuchAsElements: 25,
			},
		}
		err2 := marketDataSubscriptionManager.GetChart(mdparams)
		if err2 != nil {
			fmt.Printf("\nFailed to get chart: %v\n", err2)
		}

		//fmt.Println("tdsubs: ", tradingClientSubscriptionManager.GetActiveSubscriptions())
		//fmt.Println("mdsubs: ", marketDataSubscriptionManager.GetActiveSubscriptions())
	}()

	fmt.Println("tdsubs: ", tradingClientSubscriptionManager.GetActiveSubscriptions())
	fmt.Println("mdsubs: ", marketDataSubscriptionManager.GetActiveSubscriptions())

	// Keep the program running
	select {}
}
