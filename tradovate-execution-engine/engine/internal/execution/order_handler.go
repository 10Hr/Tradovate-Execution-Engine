package execution

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"tradovate-execution-engine/engine/config"
	"tradovate-execution-engine/engine/internal/auth"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/models"
	"tradovate-execution-engine/engine/internal/portfolio"
	"tradovate-execution-engine/engine/internal/risk"
	"tradovate-execution-engine/engine/internal/tradovate"
)

// NewOrderManager creates a new order manager
func NewOrderManager(config *config.Config, log *logger.Logger) *OrderManager {
	return &OrderManager{
		orders:         make(map[string]*models.Order),
		riskManager:    risk.NewRiskManager(config, log),
		config:         config,
		log:            log,
		orderIDCounter: 0,
	}
}

// SetPositionManager sets the position manager for the order manager
func (om *OrderManager) SetPositionManager(pm *portfolio.PositionManager) {
	om.Mu.Lock()
	defer om.Mu.Unlock()
	om.positionManager = pm
}

// HandleRawOrderEvent processes raw order events from the API
func (om *OrderManager) HandleRawOrderEvent(data json.RawMessage) {
	// Try parsing as list first
	var events []tradovate.APIOrderEvent
	if err := json.Unmarshal(data, &events); err != nil {
		// Try parsing as single object
		var singleEvent tradovate.APIOrderEvent
		if err2 := json.Unmarshal(data, &singleEvent); err2 != nil {
			om.log.Warnf("Failed to parse order event: %v", err)
			return
		}
		events = append(events, singleEvent)
	}

	for _, event := range events {
		om.processOrderEvent(event)
	}
}

func (om *OrderManager) processOrderEvent(event tradovate.APIOrderEvent) {
	om.log.Infof("Received Order Event: ID=%d Status=%s Filled=%d @ %.2f",
		event.OrderID, event.OrdStatus, event.FilledQty, event.AvgFillPrice)

	// We need to map external ID (int) to our internal ID (string)
	// Iterate through orders to find matching ExternalID
	// This is O(N) but N is small. If N grows, we need a reverse map.
	var order *models.Order

	om.Mu.RLock()
	for _, o := range om.orders {
		if o.ExternalID == fmt.Sprintf("%d", event.OrderID) || o.ExternalID == fmt.Sprintf("%d", event.ID) {
			order = o
			break
		}
	}
	om.Mu.RUnlock()

	if order == nil {
		// It might be an order originating from outside this engine (e.g. mobile app)
		// We should probably track it too, but for now just log
		om.log.Debugf("Order event for unknown order ID: %d", event.OrderID)
		return
	}

	// Update Status
	switch event.OrdStatus {
	case "Filled":
		// Calculate fill qty for this specific event?
		// Tradovate sends "cumQty".
		// If we want incremental fill, we need to track previous cumQty.
		// For simplicity, let's assume fully filled or just update to the new state.

		if order.Status != models.StatusFilled {
			// This is a new fill state
			// Use ProcessFill to handle PnL and Position updates
			// But ProcessFill expects incremental quantity and price of this fill.
			// Tradovate gives us Cumulative Qty and Avg Price.

			// Delta Qty = Event.FilledQty - Order.FilledQty
			deltaQty := event.FilledQty - order.FilledQty

			if deltaQty > 0 {
				om.ProcessFill(order.ID, event.AvgFillPrice, deltaQty)
			}
		}

	case "Rejected":
		om.Mu.Lock()
		canRetry := order.RetryCount < om.config.Risk.MaxOrderRetries
		if canRetry {
			order.RetryCount++
			om.log.Warnf("Order %s rejected, retrying (Attempt %d/%d)... Reason: %s",
				order.ID, order.RetryCount, om.config.Risk.MaxOrderRetries, event.RejectReason)
		}
		om.Mu.Unlock()

		if canRetry {
			// Update status to pending before resubmitting
			om.updateOrderStatus(order.ID, models.StatusPending, fmt.Sprintf("Retrying after rejection (Attempt %d)", order.RetryCount))
			om.log.Printf("ORDER RETRY [%s]: Resubmitting after rejection", order.ID)

			// Try to resubmit
			if err := om.submitOrderToExchange(order); err != nil {
				om.updateOrderStatus(order.ID, models.StatusRejected, "Retry failed: "+err.Error())
			}
		} else {
			om.updateOrderStatus(order.ID, models.StatusRejected, event.RejectReason+" "+event.Text)
			om.log.Errorf("ORDER FAILED [%s]: Max retries reached or unrecoverable error. Reason: %s", order.ID, event.RejectReason)
		}

	case "Cancelled", "Canceled":
		om.updateOrderStatus(order.ID, models.StatusCanceled, event.Text)

	case "Working":
		om.updateOrderStatus(order.ID, models.StatusSubmitted, "")

	default:
		// Pending, Suspended, etc.
		// Map to StatusPending?
		// om.updateOrderStatus(order.ID, StatusPending, "")
	}
}

// HandlePositionUpdate processes real-time position updates
func (om *OrderManager) HandlePositionUpdate(pos tradovate.APIPosition) {
	if om.positionManager != nil {
		om.positionManager.HandlePositionUpdate(pos)
	}
}

// SubmitMarketOrder submits a market order
func (om *OrderManager) SubmitMarketOrder(symbol string, side models.OrderSide, quantity int) (*models.Order, error) {
	om.Mu.Lock()

	// Generate order ID
	om.orderIDCounter++
	orderID := fmt.Sprintf("ORD-%s-%d-%d", symbol, time.Now().Unix(), om.orderIDCounter)

	order := &models.Order{
		ID:          orderID,
		Symbol:      symbol,
		Side:        side,
		Type:        models.TypeMarket,
		Quantity:    quantity,
		Price:       0, // Market order
		Status:      models.StatusPending,
		SubmittedAt: time.Now(),
	}

	om.orders[orderID] = order
	om.Mu.Unlock()

	om.log.Infof("Created market order: %s %s %d %s", orderID, side, quantity, symbol)

	// Check risk before submitting
	var currentPosition *portfolio.PositionPL
	if om.positionManager != nil {
		pos := om.positionManager.Pls[symbol]
		currentPosition = pos
	}

	workingBuyQty, workingSellQty := om.getWorkingQuantities(orderID)
	if err := om.riskManager.CheckOrderRisk(order, currentPosition, workingBuyQty, workingSellQty); err != nil {
		om.updateOrderStatus(orderID, models.StatusRejected, err.Error())
		return order, fmt.Errorf("risk check failed: %w", err)
	}

	// Submit order (this will eventually call Tradovate API)
	if err := om.submitOrderToExchange(order); err != nil {
		om.updateOrderStatus(orderID, models.StatusFailed, err.Error())
		return order, err
	}

	om.updateOrderStatus(orderID, models.StatusSubmitted, "")
	return order, nil
}

// // SubmitLimitOrder submits a limit order
// func (om *OrderManager) SubmitLimitOrder(symbol string, side OrderSide, quantity int, price float64) (*models.Order, error) {
// 	om.Mu.Lock()

// 	om.orderIDCounter++
// 	orderID := fmt.Sprintf("ORD-%s-%d-%d", symbol, time.Now().Unix(), om.orderIDCounter)

// 	order := &Order{
// 		ID:          orderID,
// 		Symbol:      symbol,
// 		Side:        side,
// 		Type:        TypeLimit,
// 		Quantity:    quantity,
// 		Price:       price,
// 		Status:      StatusPending,
// 		SubmittedAt: time.Now(),
// 	}

// 	om.orders[orderID] = order
// 	om.Mu.Unlock()

// 	om.log.Infof("Created limit order: %s %s %d %s @ %.2f", orderID, side, quantity, symbol, price)

// 	// Check risk before submitting
// 	currentPosition := om.positionTracker.GetPosition()
// 	if err := om.riskManager.CheckOrderRisk(order, &currentPosition); err != nil {
// 		om.updateOrderStatus(orderID, StatusRejected, err.Error())
// 		return order, fmt.Errorf("risk check failed: %w", err)
// 	}

// 	// Submit order
// 	if err := om.submitOrderToExchange(order); err != nil {
// 		om.updateOrderStatus(orderID, StatusFailed, err.Error())
// 		return order, err
// 	}

// 	om.updateOrderStatus(orderID, StatusSubmitted, "")
// 	return order, nil
// }

// getWorkingQuantities returns the total quantity of working buy and sell orders, excluding the specified order ID
func (om *OrderManager) getWorkingQuantities(excludeOrderID string) (int, int) {
	om.Mu.RLock()
	defer om.Mu.RUnlock()

	buyQty := 0
	sellQty := 0

	for _, order := range om.orders {
		if order.ID == excludeOrderID {
			continue
		}
		// Consider Pending and Submitted orders as working
		if order.Status == models.StatusPending || order.Status == models.StatusSubmitted {
			switch order.Side {
			case models.SideBuy:
				buyQty += order.Quantity
			case models.SideSell:
				sellQty += order.Quantity
			}
		}
	}
	return buyQty, sellQty
}

// submitOrderToExchange submits order to the exchange (Tradovate API)
func (om *OrderManager) submitOrderToExchange(order *models.Order) error {
	//om.log.Infof("Submitting order %s to exchange...", order.ID)
	om.log.Infof("Submitting order %s to exchange...", order.ID)

	//fmt.Printf("Submitting order %s to exchange...\n", order.ID)
	// Get TokenManager
	tm := auth.GetTokenManager()

	// Check if authenticated
	if !tm.IsAuthenticated() {
		return fmt.Errorf("not authenticated")
	}

	token, err := tm.GetAccessToken()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	accountID, err := tm.GetAccountID()
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	// 	const body = {
	//     accountSpec: yourUserName,
	//     accountId: yourAcctId,
	//     action: "Buy",
	//     symbol: "MYMM1",
	//     orderQty: 1,
	//     orderType: "Market",
	//     isAutomated: true //must be true if this isn't an order made directly by a human
	// }
	orderRequest := map[string]interface{}{
		"accountSpec": tm.GetUsername(),
		"accountId":   accountID,
		"action":      string(order.Side),
		"symbol":      order.Symbol,
		"orderQty":    order.Quantity,
		"orderType":   string(order.Type),
		"isAutomated": true,
	}

	if order.Type == models.TypeLimit {
		orderRequest["price"] = order.Price
	}

	resp, err := tm.MakeAuthenticatedRequest(
		"POST",
		"/v1/order/placeorder",
		orderRequest,
		token,
	)
	if err != nil {
		return fmt.Errorf("failed to submit order: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("order submission failed: %s", string(body))
	}

	// Parse response to get external order ID
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if orderId, ok := result["orderId"].(float64); ok {
		// API often returns numbers as float64 in generic JSON map
		order.ExternalID = fmt.Sprintf("%.0f", orderId)
	} else if orderIdStr, ok := result["orderId"].(string); ok {
		order.ExternalID = orderIdStr
	}

	//fmt.Printf("Order %s submitted successfully (External ID: %s)\n", order.ID, order.ExternalID)
	om.log.Infof("Order %s submitted successfully (External ID: %s)", order.ID, order.ExternalID)
	//om.log.Infof("Order %s submitted successfully (External ID: %s)", order.ID, order.ExternalID)
	return nil
}

// ProcessFill processes an order fill
func (om *OrderManager) ProcessFill(orderID string, fillPrice float64, fillQuantity int) error {
	om.Mu.RLock()
	order, exists := om.orders[orderID]
	om.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("order not found: %s", orderID)
	}

	om.log.Infof("Processing fill for order %s: %d @ %.2f", orderID, fillQuantity, fillPrice)

	// Update order
	om.Mu.Lock()
	order.Status = models.StatusFilled
	order.FilledAt = time.Now()
	order.FilledPrice = fillPrice
	order.FilledQty = fillQuantity
	om.Mu.Unlock()

	// Update trade count
	om.riskManager.IncrementTradeCount()

	om.log.Infof("Order %s filled: %s %d %s @ %.2f",
		orderID, order.Side, fillQuantity, order.Symbol, fillPrice)

	return nil
}

// CancelOrder cancels an order
func (om *OrderManager) CancelOrder(orderID string) error {
	om.Mu.RLock()
	order, exists := om.orders[orderID]
	om.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("order not found: %s", orderID)
	}

	if order.Status == models.StatusFilled {
		return fmt.Errorf("cannot cancel filled order: %s", orderID)
	}

	om.log.Infof("Canceling order %s...", orderID)

	// Get TokenManager
	tm := auth.GetTokenManager()

	// Check if authenticated
	if !tm.IsAuthenticated() {
		return fmt.Errorf("not authenticated")
	}

	token, err := tm.GetAccessToken()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	accountID, err := tm.GetAccountID()
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	// Tradovate expects external order ID for cancellation
	// If we don't have it (e.g. simulated or pending), we can't cancel via API
	if order.ExternalID == "" {
		om.updateOrderStatus(orderID, models.StatusCanceled, "Local cancellation (no external ID)")
		return nil
	}

	// We might need numeric orderId
	// Assuming ExternalID is the one.
	// Tradovate API for cancelorder usually requires orderId (int) and accountId
	// But let's follow the previous commented code which used orderId: order.ExternalID

	cancelRequest := map[string]interface{}{
		"orderId":   order.ExternalID,
		"accountId": accountID,
	}

	resp, err := tm.MakeAuthenticatedRequest(
		"POST",
		"/v1/order/cancelorder",
		cancelRequest,
		token,
	)
	if err != nil {
		return fmt.Errorf("failed to cancel order: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		om.log.Warnf("Cancel failed: %s", string(body))
		// Don't error out, just log it, as order might already be filled/cancelled
	}

	om.updateOrderStatus(orderID, models.StatusCanceled, "")
	om.log.Infof("Order %s canceled", orderID)

	return nil
}

// updateOrderStatus updates an order's status
func (om *OrderManager) updateOrderStatus(orderID string, status models.OrderStatus, reason string) {
	om.Mu.Lock()
	defer om.Mu.Unlock()

	if order, exists := om.orders[orderID]; exists {
		order.Status = status
		if reason != "" {
			order.RejectReason = reason
			om.log.Warnf("Order %s status: %s - %s", orderID, status, reason)
		} else {
			om.log.Infof("Order %s status: %s", orderID, status)
		}
	}
}

// GetOrder returns an order by ID
func (om *OrderManager) GetOrder(orderID string) (*models.Order, error) {
	om.Mu.RLock()
	defer om.Mu.RUnlock()

	order, exists := om.orders[orderID]
	if !exists {
		return nil, fmt.Errorf("order not found: %s", orderID)
	}

	return order, nil
}

// GetAllOrders returns all orders
func (om *OrderManager) GetAllOrders() []*models.Order {
	om.Mu.RLock()
	defer om.Mu.RUnlock()

	orders := make([]*models.Order, 0, len(om.orders))
	for _, order := range om.orders {
		orders = append(orders, order)
	}

	return orders
}

// GetPosition returns the current position for the main symbol
func (om *OrderManager) GetPosition(symbol string) portfolio.PositionPL {
	if om.positionManager != nil {
		om.positionManager.Mu.RLock()
		defer om.positionManager.Mu.RUnlock()
		if pos, ok := om.positionManager.Pls[symbol]; ok {
			return *pos
		}
	}
	return portfolio.PositionPL{Name: symbol}
}

// GetDailyPnL returns the total daily PnL from PositionManager
func (om *OrderManager) GetDailyPnL() float64 {
	if om.positionManager != nil {
		return om.positionManager.GetTotalPL()
	}
	return 0
}

// UpdatePrice is now a no-op as PositionManager handles quote updates directly
func (om *OrderManager) UpdatePrice(price float64) {
}

// Reset resets the order manager
func (om *OrderManager) Reset() {
	om.Mu.Lock()
	om.orders = make(map[string]*models.Order)
	om.orderIDCounter = 0
	om.Mu.Unlock()

	om.log.Info("Order manager reset")
}
