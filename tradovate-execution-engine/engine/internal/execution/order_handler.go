package execution

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"tradovate-execution-engine/engine/internal/auth"
	"tradovate-execution-engine/engine/internal/logger"
)

// APIPosition represents the Tradovate position response
type APIPosition struct {
	ID          int     `json:"id"`
	ContractID  int     `json:"contractId"`
	NetPos      int     `json:"netPos"`
	NetPrice    float64 `json:"netPrice"`
	BoughtValue float64 `json:"boughtValue"`
	SoldValue   float64 `json:"soldValue"`
}

// OrderManager handles order submission and tracking
type OrderManager struct {
	mu              sync.RWMutex
	orders          map[string]*Order // Map of order ID to order
	positionTracker *PositionTracker
	riskManager     *RiskManager
	config          *Config
	log             *logger.Logger
	orderIDCounter  int
}

// NewOrderManager creates a new order manager
func NewOrderManager(symbol string, config *Config, log *logger.Logger) *OrderManager {
	return &OrderManager{
		orders:          make(map[string]*Order),
		positionTracker: NewPositionTracker(symbol, log),
		riskManager:     NewRiskManager(config, log),
		config:          config,
		log:             log,
		orderIDCounter:  0,
	}
}

// HandlePositionUpdate processes real-time position updates
func (om *OrderManager) HandlePositionUpdate(pos APIPosition) {
	om.log.Infof("Processing Position Update: NetPos %d @ %.2f", pos.NetPos, pos.NetPrice)

	// Realized PnL Calculation:
	// Cash Flow = SoldValue - BoughtValue
	// Cost of Open Position = NetPos * NetPrice
	// Realized PnL = Cash Flow + Cost of Open Position

	realizedPnLPoints := (pos.SoldValue - pos.BoughtValue) + (float64(pos.NetPos) * pos.NetPrice)

	// Multiplier (Hardcoded 50 for now, ideally dynamic)
	multiplier := 50.0
	realizedPnL := realizedPnLPoints * multiplier

	om.positionTracker.UpdatePositionFromAPI(pos.NetPos, pos.NetPrice, realizedPnL)
	om.riskManager.SetDailyPnL(realizedPnL)
}

// SubmitMarketOrder submits a market order
func (om *OrderManager) SubmitMarketOrder(symbol string, side OrderSide, quantity int) (*Order, error) {
	om.mu.Lock()

	// Generate order ID
	om.orderIDCounter++
	orderID := fmt.Sprintf("ORD-%s-%d-%d", symbol, time.Now().Unix(), om.orderIDCounter)

	order := &Order{
		ID:          orderID,
		Symbol:      symbol,
		Side:        side,
		Type:        TypeMarket,
		Quantity:    quantity,
		Price:       0, // Market order
		Status:      StatusPending,
		SubmittedAt: time.Now(),
	}

	om.orders[orderID] = order
	om.mu.Unlock()

	//fmt.Printf("Created market order: %s %s %d %s\n", orderID, side, quantity, symbol)
	om.log.Infof("Created market order: %s %s %d %s", orderID, side, quantity, symbol)
	// Check risk before submitting
	currentPosition := om.positionTracker.GetPosition()
	if err := om.riskManager.CheckOrderRisk(order, &currentPosition); err != nil {
		om.updateOrderStatus(orderID, StatusRejected, err.Error())
		return order, fmt.Errorf("risk check failed: %w", err)
	}

	// Submit order (this will eventually call Tradovate API)
	if err := om.submitOrderToExchange(order); err != nil {
		om.updateOrderStatus(orderID, StatusFailed, err.Error())
		return order, err
	}

	om.updateOrderStatus(orderID, StatusSubmitted, "")
	return order, nil
}

// // SubmitLimitOrder submits a limit order
// func (om *OrderManager) SubmitLimitOrder(symbol string, side OrderSide, quantity int, price float64) (*Order, error) {
// 	om.mu.Lock()

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
// 	om.mu.Unlock()

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

// submitOrderToExchange submits order to the exchange (Tradovate API)
func (om *OrderManager) submitOrderToExchange(order *Order) error {
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

	if order.Type == TypeLimit {
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
	om.mu.RLock()
	order, exists := om.orders[orderID]
	om.mu.RUnlock()

	if !exists {
		return fmt.Errorf("order not found: %s", orderID)
	}

	om.log.Infof("Processing fill for order %s: %d @ %.2f", orderID, fillQuantity, fillPrice)

	// Update order
	om.mu.Lock()
	order.Status = StatusFilled
	order.FilledAt = time.Now()
	order.FilledPrice = fillPrice
	order.FilledQty = fillQuantity
	om.mu.Unlock()

	// Update position
	fill := &Fill{
		OrderID:   orderID,
		Price:     fillPrice,
		Quantity:  fillQuantity,
		Timestamp: time.Now(),
	}
	realizedPnL := om.positionTracker.UpdateFill(fill, order.Side)

	// Update trade count
	om.riskManager.IncrementTradeCount()

	// Update daily PnL if position was reduced/closed
	om.riskManager.UpdatePnL(realizedPnL)

	om.log.Infof("Order %s filled: %s %d %s @ %.2f",
		orderID, order.Side, fillQuantity, order.Symbol, fillPrice)

	return nil
}

// CancelOrder cancels an order
func (om *OrderManager) CancelOrder(orderID string) error {
	om.mu.RLock()
	order, exists := om.orders[orderID]
	om.mu.RUnlock()

	if !exists {
		return fmt.Errorf("order not found: %s", orderID)
	}

	if order.Status == StatusFilled {
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
		om.updateOrderStatus(orderID, StatusCanceled, "Local cancellation (no external ID)")
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

	om.updateOrderStatus(orderID, StatusCanceled, "")
	om.log.Infof("Order %s canceled", orderID)

	return nil
}

// updateOrderStatus updates an order's status
func (om *OrderManager) updateOrderStatus(orderID string, status OrderStatus, reason string) {
	om.mu.Lock()
	defer om.mu.Unlock()

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
func (om *OrderManager) GetOrder(orderID string) (*Order, error) {
	om.mu.RLock()
	defer om.mu.RUnlock()

	order, exists := om.orders[orderID]
	if !exists {
		return nil, fmt.Errorf("order not found: %s", orderID)
	}

	return order, nil
}

// GetAllOrders returns all orders
func (om *OrderManager) GetAllOrders() []*Order {
	om.mu.RLock()
	defer om.mu.RUnlock()

	orders := make([]*Order, 0, len(om.orders))
	for _, order := range om.orders {
		orders = append(orders, order)
	}

	return orders
}

// GetPosition returns the current position
func (om *OrderManager) GetPosition() Position {
	return om.positionTracker.GetPosition()
}

// GetDailyPnL returns the daily PnL
func (om *OrderManager) GetDailyPnL() float64 {
	return om.riskManager.GetDailyPnL()
}

// UpdatePrice updates the current market price for PnL calculations
func (om *OrderManager) UpdatePrice(price float64) {
	om.positionTracker.UpdatePrice(price)
}

// Reset resets the order manager (clears all orders and positions)
func (om *OrderManager) Reset() {
	om.mu.Lock()
	om.orders = make(map[string]*Order)
	om.orderIDCounter = 0
	om.mu.Unlock()

	om.positionTracker.Reset()
	om.log.Info("Order manager reset")
}
