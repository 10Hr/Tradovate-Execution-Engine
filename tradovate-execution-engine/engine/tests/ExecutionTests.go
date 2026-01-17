package main

import (
	"fmt"

	"tradovate-execution-engine/engine/internal/execution"
	"tradovate-execution-engine/engine/internal/logger"
)

func main() {
	// Initialize logger
	log := logger.NewLogger(1000)
	log.Info("=== Order Management System Test ===")
	log.Info("")

	// Test 1: Configuration
	log.Info("TEST 1: Creating configuration")
	config := execution.DefaultConfig()
	log.Infof("Default config - Max Contracts: %d, Daily Loss Limit: $%.2f",
		config.MaxContracts, config.DailyLossLimit)
	log.Info("")

	// Test 2: Create Order Manager
	log.Info("TEST 2: Creating Order Manager")
	symbol := "ESH5" // E-mini S&P 500 March 2025
	orderManager := execution.NewOrderManager(symbol, config, log)
	log.Info("Order Manager created successfully")
	log.Info("")

	// Test 3: Check initial position
	log.Info("TEST 3: Checking initial position")
	position := orderManager.GetPosition()
	printPosition(position, log)
	log.Info("")

	// Test 4: Submit a market buy order
	log.Info("TEST 4: Submitting market BUY order")
	buyOrder, err := orderManager.SubmitMarketOrder(symbol, execution.SideBuy, 1)
	if err != nil {
		log.Errorf("Failed to submit buy order: %v", err)
	} else {
		log.Infof("Buy order submitted: %s", buyOrder.ID)
		printOrder(buyOrder, log)
	}
	log.Info("")

	// Test 5: Simulate fill for buy order
	log.Info("TEST 5: Simulating fill for buy order")
	fillPrice := 5850.25
	err = orderManager.ProcessFill(buyOrder.ID, fillPrice, 1)
	if err != nil {
		log.Errorf("Failed to process fill: %v", err)
	} else {
		log.Info("Fill processed successfully")
		position = orderManager.GetPosition()
		printPosition(position, log)
	}
	log.Info("")

	// Test 6: Update market price to simulate profit
	log.Info("TEST 6: Updating market price (simulating profit)")
	newPrice := 5855.50
	orderManager.UpdatePrice(newPrice)
	position = orderManager.GetPosition()
	printPosition(position, log)
	log.Infof("Price moved from %.2f to %.2f (gain of $%.2f)", fillPrice, newPrice, newPrice-fillPrice)
	log.Info("")

	// Test 7: Submit a sell order to close position
	log.Info("TEST 7: Submitting market SELL order to close position")
	sellOrder, err := orderManager.SubmitMarketOrder(symbol, execution.SideSell, 1)
	if err != nil {
		log.Errorf("Failed to submit sell order: %v", err)
	} else {
		log.Infof("Sell order submitted: %s", sellOrder.ID)
		printOrder(sellOrder, log)
	}
	log.Info("")

	// Test 8: Simulate fill for sell order
	log.Info("TEST 8: Simulating fill for sell order")
	exitPrice := 5856.00
	err = orderManager.ProcessFill(sellOrder.ID, exitPrice, 1)
	if err != nil {
		log.Errorf("Failed to process fill: %v", err)
	} else {
		log.Info("Fill processed successfully")
		position = orderManager.GetPosition()
		printPosition(position, log)
		log.Infof("Trade P&L: $%.2f", exitPrice-fillPrice)
	}
	log.Info("")

	// Test 9: Try to exceed max contracts (should fail)
	log.Info("TEST 9: Testing risk check - exceeding max contracts")
	_, err = orderManager.SubmitMarketOrder(symbol, execution.SideBuy, 2) // Trying to buy 2 contracts
	if err != nil {
		log.Infof("Order correctly rejected: %v", err)
	} else {
		log.Error("Order should have been rejected but wasn't!")
	}
	log.Info("")

	// Test 10: Simulate daily loss limit
	log.Info("TEST 10: Testing daily loss limit")
	testDailyLossLimit(orderManager, symbol, log)
	log.Info("")

	// Test 11: Submit and cancel limit order
	//log.Info("TEST 11: Submitting and canceling limit order")
	//limitOrder, err := orderManager.SubmitLimitOrder(symbol, execution.SideBuy, 1, 5845.00)
	// if err != nil {
	// 	log.Errorf("Failed to submit limit order: %v", err)
	// } else {
	// 	log.Infof("Limit order submitted: %s", limitOrder.ID)
	// 	printOrder(limitOrder, log)

	// 	// Cancel the order
	// 	time.Sleep(100 * time.Millisecond)
	// 	err = orderManager.CancelOrder(limitOrder.ID)
	// 	if err != nil {
	// 		log.Errorf("Failed to cancel order: %v", err)
	// 	} else {
	// 		log.Info("Order canceled successfully")
	// 	}
	// }
	// log.Info("")

	// Test 12: Get all orders
	log.Info("TEST 12: Retrieving all orders")
	allOrders := orderManager.GetAllOrders()
	log.Infof("Total orders: %d", len(allOrders))
	for i, order := range allOrders {
		log.Infof("Order %d: %s - %s - %s", i+1, order.ID, order.Status, order.Side)
	}
	log.Info("")

	// Test 13: Daily statistics
	log.Info("TEST 13: Daily statistics")
	dailyPnL := orderManager.GetDailyPnL()
	log.Infof("Daily P&L: $%.2f", dailyPnL)
	log.Info("")

	// Test 14: Reset order manager
	log.Info("TEST 14: Resetting order manager")
	orderManager.Reset()
	position = orderManager.GetPosition()
	printPosition(position, log)
	allOrders = orderManager.GetAllOrders()
	log.Infof("Orders after reset: %d", len(allOrders))
	log.Info("")

	// Print full log
	log.Info("=== FULL TEST LOG ===")
	fmt.Println(log.ExportToString())
}

func printPosition(pos execution.Position, log *logger.Logger) {
	if pos.IsFlat() {
		log.Info("Position: FLAT")
	} else {
		direction := "LONG"
		if pos.IsShort() {
			direction = "SHORT"
		}
		log.Infof("Position: %s %d contracts", direction, abs(pos.Quantity))
		log.Infof("  Entry Price: $%.2f", pos.EntryPrice)
		log.Infof("  Current Price: $%.2f", pos.CurrentPrice)
		log.Infof("  Unrealized P&L: $%.2f", pos.UnrealizedPnL)
		log.Infof("  Realized P&L: $%.2f", pos.RealizedPnL)
	}
}

func printOrder(order *execution.Order, log *logger.Logger) {
	log.Infof("  Order ID: %s", order.ID)
	log.Infof("  Symbol: %s", order.Symbol)
	log.Infof("  Side: %s", order.Side)
	log.Infof("  Type: %s", order.Type)
	log.Infof("  Quantity: %d", order.Quantity)
	if order.Type == execution.TypeLimit {
		log.Infof("  Limit Price: $%.2f", order.Price)
	}
	log.Infof("  Status: %s", order.Status)
}

func testDailyLossLimit(om *execution.OrderManager, symbol string, log *logger.Logger) {
	// Create a new order manager with a low loss limit
	lowLimitConfig := execution.NewConfig(1, 50.0) // $50 loss limit
	testOM := execution.NewOrderManager(symbol, lowLimitConfig, log)

	// Simulate losing trade
	log.Info("Simulating losing trade...")
	order, _ := testOM.SubmitMarketOrder(symbol, execution.SideBuy, 1)
	testOM.ProcessFill(order.ID, 5850.00, 1)

	// Close at a loss
	exitOrder, _ := testOM.SubmitMarketOrder(symbol, execution.SideSell, 1)
	testOM.ProcessFill(exitOrder.ID, 5800.00, 1) // $50 loss

	dailyPnL := testOM.GetDailyPnL()
	log.Infof("Daily P&L after loss: $%.2f", dailyPnL)

	// Try to submit another order (should fail due to loss limit)
	_, err := testOM.SubmitMarketOrder(symbol, execution.SideBuy, 1)
	if err != nil {
		log.Infof("Order correctly blocked by loss limit: %v", err)
	} else {
		log.Error("Order should have been blocked!")
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
