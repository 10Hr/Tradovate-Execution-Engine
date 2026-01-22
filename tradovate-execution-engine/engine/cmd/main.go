package main

import (
	"fmt"
	"os"
	"time"
	"tradovate-execution-engine/engine/config"
	"tradovate-execution-engine/engine/internal/auth"
	"tradovate-execution-engine/engine/internal/execution"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/marketdata"
	"tradovate-execution-engine/engine/internal/portfolio"
	"tradovate-execution-engine/engine/internal/tradovate"
	_ "tradovate-execution-engine/engine/strategies"
)

const (
	symbol     = "MESH6"
	configFile = "config/config.json"
)

func main() {

	mainLog := logger.NewLogger(500)
	orderLog := logger.NewLogger(500)

	// Ensure config directory exists
	_ = os.MkdirAll("../../config", 0755)

	// Initialize Config for OrderManager
	config, err := config.LoadOrCreateConfig()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}

	// Initialize OrderManager
	orderManager := execution.NewOrderManager(config, orderLog)

	// //Pass to UI:
	// p := tea.NewProgram(UI.InitialModel(mainLog, orderLog, orderManager), tea.WithAltScreen())
	// if _, err := p.Run(); err != nil {
	// 	fmt.Printf("Error: %v\n", err)
	// }
	// return

	//
	//	PROGRAM CONTROL FLOW - USE THIS FOR WHEN
	//

	// Get the global token manager
	tm := auth.GetTokenManager()

	// // Set credentials
	tm.SetCredentials(
		config.Tradovate.AppID,
		config.Tradovate.AppVersion,
		config.Tradovate.Chl,
		config.Tradovate.Cid,
		config.Tradovate.DeviceID,
		config.Tradovate.Environment,
		config.Tradovate.Username,
		config.Tradovate.Password,
		config.Tradovate.Sec,
		config.Tradovate.Enc,
	)

	// // Authenticate
	if err := tm.Authenticate(); err != nil {
		fmt.Println("Authentication error:", err)
		return
	}
	fmt.Println("Authentication Successful")

	// // Now you can use the token in other parts of your code
	accessToken, err := tm.GetAccessToken()
	if err != nil {
		fmt.Println("Error getting token:", err)
		return
	}

	fmt.Println("\nAuth token aquired")

	var mdToken string
	mdToken, err = tm.GetMDAccessToken()

	if err != nil {
		fmt.Println("Error getting market data token:", err)
		return
	}

	fmt.Println("\nMarket data token aquired")

	marketDataClient := tradovate.NewTradovateWebSocketClient(mdToken, config.Tradovate.Environment, "md")
	marketDataClient.SetLogger(mainLog)

	// Trading Client
	tradingClient := tradovate.NewTradovateWebSocketClient(accessToken, config.Tradovate.Environment, "")
	tradingClient.SetLogger(mainLog)

	// Create Subscribers
	mdSubscriber := tradovate.NewDataSubscriber(marketDataClient)
	mdSubscriber.SetLogger(mainLog)

	tradingSubscriber := tradovate.NewDataSubscriber(tradingClient)
	tradingSubscriber.SetLogger(mainLog)

	// Set Handlers
	marketDataClient.SetMessageHandler(mdSubscriber.HandleEvent)
	tradingClient.SetMessageHandler(tradingSubscriber.HandleEvent)

	var availableStrats []string
	availableStrats = execution.GetAvailableStrategies()
	fmt.Printf("Discovered %d registered strategies: %v\n", len(availableStrats), availableStrats)

	strat, err := execution.CreateStrategy("ma_crossover")

	fmt.Println("\nStrategy", strat.GetParams())

	strat.SetParam("fast_length", "2")
	strat.SetParam("slow_length", "5")

	if err := strat.Init(orderManager); err != nil {
		fmt.Println("Failed to initialize strategy: " + err.Error())
		return
	}

	params := strat.GetParams()

	fmt.Println(params)

	var symbol string
	for _, param := range params {
		if param.Name == "symbol" {
			symbol = param.Value // Get the VALUE which is "MESH6"
			break
		}
	}

	// Create SMAs BEFORE the handler (outside, at the same level as AddChartHandler)
	//fastSMA := indicators.NewSMA(5, indicators.OnBarClose)
	//slowSMA := indicators.NewSMA(15, indicators.OnBarClose)
	// Track last bar to avoid exact duplicates
	type LastBar struct {
		Timestamp string
		Close     float64
	}
	var lastBar LastBar

	var historicalLoaded bool
	mdSubscriber.AddChartHandler(func(update marketdata.ChartUpdate) {
		fmt.Printf("\n🔔 CHART UPDATE RECEIVED at %s\n", time.Now().Format("15:04:05"))
		fmt.Printf("📊 Chart handler called with %d charts\n", len(update.Charts))

		for _, chart := range update.Charts {
			fmt.Printf("Chart ID: %d | Bars: %d | Ticks: %d | EOH: %v\n",
				chart.ID, len(chart.Bars), len(chart.Ticks), chart.EOH)

			// Check for end of history marker
			if chart.EOH {
				fmt.Println("✅ End of historical data - now receiving live updates")
				fmt.Printf("📊 SMAs initialized - Fast: %.2f | Slow: %.2f\n",
					strat.GetMetrics()["Fast SMA"],
					strat.GetMetrics()["Slow SMA"])

				// Enable strategy for live trading
				if s, ok := strat.(interface{ SetEnabled(bool) }); ok {
					s.SetEnabled(true)
					fmt.Println("🚀 Strategy enabled for LIVE trading")
				}

				historicalLoaded = true
				continue
			}

			fmt.Printf("\n=== Chart ID: %d ===\n", chart.ID)
			fmt.Printf("Number of bars: %d\n", len(chart.Bars))

			if !historicalLoaded {
				// Process each bar
				for i, bar := range chart.Bars {
					// Skip only exact duplicates (same timestamp AND same close price)
					if bar.Timestamp == lastBar.Timestamp && bar.Close == lastBar.Close {
						fmt.Printf("  ⏭️  Skipping duplicate bar at %s\n", bar.Timestamp)
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

					// Parse and format time for display
					t, err := time.Parse("2006-01-02T15:04Z", bar.Timestamp)
					humanTime := bar.Timestamp
					if err == nil {
						humanTime = t.Local().Format("Jan 02 3:04 PM")
					}

					// Print bar with SMA values
					fmt.Printf("Bar %d: %s | C=%.2f | Fast SMA(5)=%.2f | Slow SMA(15)=%.2f\n",
						i+1,
						humanTime,
						bar.Close,
						strat.GetMetrics()["Fast SMA"],
						strat.GetMetrics()["Slow SMA"])
				}

				fmt.Printf("📊 Current Fast SMA: %.2f | Slow SMA: %.2f\n",
					strat.GetMetrics()["Fast SMA"],
					strat.GetMetrics()["Slow SMA"])
				// fastSMA.CurrentValue(),
				// slowSMA.CurrentValue())
			}
		}
	})

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

	mdSubscriber.AddQuoteHandler(func(quote marketdata.Quote) {

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
					//fmt.Println("LiveBarCount: ", livebarcounter)
					//fmt.Printf("   Fast SMA: %.2f | Slow SMA: %.2f\n", fastValue, slowValue)

					// fmt.Printf("   Fast SMA: %.2f | Slow SMA: %.2f\n",
					// 	strat.GetMetrics()["Fast SMA"],
					// 	strat.GetMetrics()["Slow SMA"])
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

	go func() {
		mdSubscriber.Connect()
		mdSubscriber.SubscribeQuote(symbol)

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
		err2 := mdSubscriber.GetChart(mdparams)
		if err2 != nil {
			fmt.Printf("\nFailed to get chart: %v\n", err2)
		}

		// Add this heartbeat check
		// ticker := time.NewTicker(10 * time.Second)
		// go func() {
		// 	for range ticker.C {
		// 		fmt.Printf("⏰ Heartbeat: %s - WS Connected: %v EST\n",
		// 			time.Now().Format("15:04:05"),
		// 			mdClient.IsConnected())
		// 	}
		// }()
	}()

	//Initialize PositionManager

	userID := tm.GetUserID()

	//Create and start tracker reusing connections

	tracker := portfolio.NewPortfolioTracker(tradingClient, marketDataClient, userID, mainLog)

	if err := tracker.Start(config.Tradovate.Environment); err != nil {

		fmt.Errorf("Failed to start PortfolioTracker: %v", err)
	}

	// Keep the program running
	select {}

	// //
	// // Get account ID first
	// //
	// resp, err := tm.MakeAuthenticatedRequest("POST", "/v1/account/list", nil, token)
	// if err != nil {
	// 	fmt.Println("Request error:", err)
	// 	return
	// }
	// defer resp.Body.Close()

	// body, _ := io.ReadAll(resp.Body)
	// fmt.Println("\nAccount Information")
	// fmt.Printf("Status: %s\n", resp.Status)

	// var accounts []map[string]interface{}
	// if err := json.Unmarshal(body, &accounts); err != nil {
	// 	fmt.Println("Error parsing accounts:", err)
	// 	return
	// }

	// if len(accounts) == 0 {
	// 	fmt.Println("No accounts found")
	// 	return
	// }

	// // Get the first account ID
	// accountID := int(accounts[0]["id"].(float64))
	// fmt.Printf("Using account ID: %d\n", accountID)

	// prettyJSON, _ := json.MarshalIndent(accounts, "   ", "  ")
	// fmt.Println(string(prettyJSON))

	// fmt.Println("\nTesting market data...")

	// // Search for contracts by name
	// var data2 *http.Response
	// data2, err2 := tm.MakeAuthenticatedRequest("POST", "/contract/find?name="+symbol, nil, mdToken)
	// if err2 != nil {
	// 	fmt.Printf("Error: %v\n", err2)
	// 	return
	// }
	// defer data2.Body.Close()

	// body2, _ := io.ReadAll(data2.Body)
	// fmt.Println("\nMarketData API call response:")
	// fmt.Printf("Status: %s\n", data2.Status)

	// var result2 interface{}
	// if err := json.Unmarshal(body2, &result2); err == nil {
	// 	prettyJSON, _ := json.MarshalIndent(result2, "   ", "  ")
	// 	fmt.Println(string(prettyJSON))
	// }

	// // Create WebSocket client
	// wsClient := tradovate.NewTradovateWebSocketClient(token, "demo")

	// // Create market data components
	// subscriber := tradovate.NewDataSubscriber(wsClient)

	// // Set up composite event routing for both market data and account updates

	// // Connect
	// if err := wsClient.Connect(); err != nil {
	// 	fmt.Printf("Connection error: %v\n", err)
	// 	return
	// }

	// // Subscribe to market data
	// subscriber.SubscribeQuote(symbol)

	// // Subscribe to account updates for live PnL
	// // fmt.Printf("\nSubscribing to account updates for account ID: %d\n", accountID)
	// wsClient.Send("user/syncrequest", map[string]interface{}{
	// 	"users": []int{accountID},
	// })

	// fmt.Println("Subscribed to live PnL updates\n")
	// execConfig := execution.DefaultConfig()
	// orderLog := logger.NewLogger(500)

	// orderManager := execution.NewOrderManager(symbol, execConfig, orderLog)
	// order, err := orderManager.SubmitMarketOrder("MESH6", execution.SideBuy, 1)
	// if err != nil {
	// 	fmt.Println("Order failed: " + err.Error())
	// 	return
	// }
	// fmt.Printf("Order submitted: %+v\n", order)
	// Keep the program running
	//select {}
}
