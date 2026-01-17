package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"tradovate-execution-engine/engine/UI"
	"tradovate-execution-engine/engine/config"
	"tradovate-execution-engine/engine/internal/api"
	"tradovate-execution-engine/engine/internal/auth"
	"tradovate-execution-engine/engine/internal/execution"
	"tradovate-execution-engine/engine/internal/logger"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	symbol     = "MESH6"
	configFile = "config.json"
)

func main() {

	mainLog := logger.NewLogger(500)
	orderLog := logger.NewLogger(500)

	// Initialize Config for OrderManager
	execConfig := execution.DefaultConfig()

	// Initialize OrderManager
	orderManager := execution.NewOrderManager(symbol, execConfig, orderLog)

	// Pass to UI:
	p := tea.NewProgram(UI.InitialModel(mainLog, orderLog, orderManager), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	return
	// Get the global token manager
	tm := auth.GetTokenManager()

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		fmt.Printf("Error loading config: %v", err)
		fmt.Println("Creating default config file...")
		if err := config.CreateDefaultConfig(configFile); err != nil {
			fmt.Printf("Error creating default config: %v", err)
			return
		}
		fmt.Printf("Default config created at %s", configFile)
		return
	}
	fmt.Println("Config Loaded.")

	// Set credentials
	tm.SetCredentials(
		cfg.Tradovate.AppID,
		cfg.Tradovate.AppVersion,
		cfg.Tradovate.Chl,
		cfg.Tradovate.Cid,
		cfg.Tradovate.DeviceID,
		cfg.Tradovate.Environment,
		cfg.Tradovate.Username,
		cfg.Tradovate.Password,
		cfg.Tradovate.Sec,
		cfg.Tradovate.Enc,
	)

	// Authenticate
	if err := tm.Authenticate(); err != nil {
		fmt.Println("Authentication error:", err)
		return
	}
	fmt.Println("Authentication Successful")

	// Now you can use the token in other parts of your code
	token, err := tm.GetAccessToken()
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

	//
	// Get account ID first
	//
	resp, err := tm.MakeAuthenticatedRequest("POST", "/v1/account/list", nil, token)
	if err != nil {
		fmt.Println("Request error:", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println("\nAccount Information")
	fmt.Printf("Status: %s\n", resp.Status)

	var accounts []map[string]interface{}
	if err := json.Unmarshal(body, &accounts); err != nil {
		fmt.Println("Error parsing accounts:", err)
		return
	}

	if len(accounts) == 0 {
		fmt.Println("No accounts found")
		return
	}

	// Get the first account ID
	accountID := int(accounts[0]["id"].(float64))
	fmt.Printf("Using account ID: %d\n", accountID)

	prettyJSON, _ := json.MarshalIndent(accounts, "   ", "  ")
	fmt.Println(string(prettyJSON))

	fmt.Println("\nTesting market data...")

	// Search for contracts by name
	var data2 *http.Response
	data2, err2 := tm.MakeAuthenticatedRequest("POST", "/contract/find?name="+symbol, nil, mdToken)
	if err2 != nil {
		fmt.Printf("Error: %v\n", err2)
		return
	}
	defer data2.Body.Close()

	body2, _ := io.ReadAll(data2.Body)
	fmt.Println("\nMarketData API call response:")
	fmt.Printf("Status: %s\n", data2.Status)

	var result2 interface{}
	if err := json.Unmarshal(body2, &result2); err == nil {
		prettyJSON, _ := json.MarshalIndent(result2, "   ", "  ")
		fmt.Println(string(prettyJSON))
	}

	// Create WebSocket client
	wsClient := api.NewTradovateWebSocketClient(token, "demo")

	// Create market data components
	subscriber := api.NewDataSubscriber(wsClient)

	// Set up composite event routing for both market data and account updates

	// Connect
	wsClient.Connect()

	// Wait for connection to be established
	time.Sleep(2 * time.Second)

	// Subscribe to market data
	subscriber.SubscribeQuote(symbol)

	// Subscribe to account updates for live PnL
	// fmt.Printf("\nSubscribing to account updates for account ID: %d\n", accountID)
	wsClient.Send("user/syncrequest", map[string]interface{}{
		"users": []int{accountID},
	})

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
	select {}
}

/*
// Example usage
func main() {

	// In your main.go or any other file:
	// mainLog := logger.NewLogger(500)
	// orderLog := logger.NewLogger(500)

	// // Initialize Config for OrderManager
	// execConfig := execution.DefaultConfig()

	// // Initialize OrderManager
	// orderManager := execution.NewOrderManager(symbol, execConfig, orderLog)

	// // Pass to UI:
	// p := tea.NewProgram(UI.InitialModel(mainLog, orderLog, orderManager), tea.WithAltScreen())
	// if _, err := p.Run(); err != nil {
	// 	fmt.Printf("Error: %v\n", err)
	// }

	// return
	//---------------------------------------

	// Get the global token manager
	tm := auth.GetTokenManager()

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		fmt.Printf("Error loading config: %v", err)
		fmt.Println("Creating default config file...")
		if err := config.CreateDefaultConfig(configFile); err != nil {
			fmt.Printf("Error creating default config: %v", err)
			return
		}
		fmt.Printf("Default config created at %s", configFile)
		return
	}
	fmt.Println("Config Loaded.")

	// Set credentials
	tm.SetCredentials(
		cfg.Tradovate.AppID,
		cfg.Tradovate.AppVersion,
		cfg.Tradovate.Chl,
		cfg.Tradovate.Cid,
		cfg.Tradovate.DeviceID,
		cfg.Tradovate.Environment,
		cfg.Tradovate.Username,
		cfg.Tradovate.Password,
		cfg.Tradovate.Sec,
		cfg.Tradovate.Enc,
	)

	// Authenticate
	if err := tm.Authenticate(); err != nil {
		fmt.Println("Authentication error:", err)
		return
	}
	fmt.Println("Authentication Successful")

	// Now you can use the token in other parts of your code
	token, err := tm.GetAccessToken()
	if err != nil {
		fmt.Println("Error getting token:", err)
		return
	}

	fmt.Println("\nAuth token aquired")
	//fmt.Println("Token:", token)

	var mdToken string
	mdToken, err = tm.GetMDAccessToken()

	if err != nil {
		fmt.Println("Error getting market data token:", err)
		return
	}

	// orderLog := logger.NewLogger(500)
	// execConfig := execution.DefaultConfig()

	// // Initialize OrderManager
	// orderManager := execution.NewOrderManager(symbol, execConfig, orderLog)
	// order, err := orderManager.SubmitMarketOrder("MESH6", execution.SideBuy, 1)
	// if err != nil {
	// 	fmt.Println("Order failed: " + err.Error())
	// 	return
	// }
	// fmt.Printf("Order submitted: %+v\n", order)

	return
	fmt.Println("\nMarket data token aquired")
	//fmt.Println("Token:", mdToken)

	//
	// Example: Make an authenticated request
	//
	resp, err := tm.MakeAuthenticatedRequest("POST", "/v1/account/list", nil, token)
	if err != nil {
		fmt.Println("Request error:", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println("\nAccount Information")
	fmt.Printf("Status: %s\n", resp.Status)

	var result interface{}
	if err := json.Unmarshal(body, &result); err == nil {
		prettyJSON, _ := json.MarshalIndent(result, "   ", "  ")
		fmt.Println(string(prettyJSON))
	}

	fmt.Println("\nTesting market data...")

	// Search for contracts by name
	var data2 *http.Response
	data2, err2 := tm.MakeAuthenticatedRequest("POST", "/contract/find?name="+symbol, nil, mdToken)
	if err2 != nil {
		fmt.Printf("Error: %v\n", err2)
		return
	}
	defer data2.Body.Close()

	body2, _ := io.ReadAll(data2.Body)
	fmt.Println("\nMarketData API call response:")
	fmt.Printf("Status: %s\n", data2.Status)

	var result2 interface{}
	if err := json.Unmarshal(body2, &result2); err == nil {
		prettyJSON, _ := json.MarshalIndent(result2, "   ", "  ")
		fmt.Println(string(prettyJSON))
	}

	// Create WebSocket client
	wsClient := auth.NewTradovateWebSocketClient(token, "demo")

	// Create market data components
	subscriber := marketdata.NewMarketDataSubscriber(wsClient)
	//historical := marketdata.NewHistoricalDataClient(wsClient)

	// Set up event routing
	wsClient.SetMessageHandler(subscriber.HandleEvent)

	// Connect
	wsClient.Connect()

	// Subscribe to real-time data
	subscriber.SubscribeQuote(symbol)

	// Request historical data
	//historical.GetTickData(symbol, start, end)

	// Keep the program running
	select {}

	// Clean disconnect (you would call this when ready to exit)
	// defer mdClient.Disconnect()
}
*/
