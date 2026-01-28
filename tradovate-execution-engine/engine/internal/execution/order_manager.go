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
)

// NewOrderManager creates a new order manager
func NewOrderManager(tm *auth.TokenManager, config *config.Config, log *logger.Logger) *OrderManager {
	return &OrderManager{
		orders:         make(map[string]*models.Order),
		tokenManager:   tm,
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

	//
	// REFACTOR TO USE
	//
	if err := om.riskManager.CheckOrderRisk(order, currentPosition); err != nil {
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

// submitOrderToExchange submits order to the exchange (Tradovate API)
func (om *OrderManager) submitOrderToExchange(order *models.Order) error {
	om.log.Infof("Submitting order %s to exchange...", order.ID)

	// Check if authenticated
	if !om.tokenManager.IsAuthenticated() {
		return fmt.Errorf("not authenticated")
	}

	token, err := om.tokenManager.GetAccessToken()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	accountID, err := om.tokenManager.GetAccountID()
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
		"accountSpec": om.tokenManager.GetUsername(),
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

	resp, err := om.tokenManager.MakeAuthenticatedRequest(
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

	om.log.Infof("Order %s submitted successfully (External ID: %s)", order.ID, order.ExternalID)
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

	// Check if authenticated
	if !om.tokenManager.IsAuthenticated() {
		return fmt.Errorf("not authenticated")
	}

	token, err := om.tokenManager.GetAccessToken()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	accountID, err := om.tokenManager.GetAccountID()
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

	resp, err := om.tokenManager.MakeAuthenticatedRequest(
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

// FlattenPositions closes all open positions
func (om *OrderManager) FlattenPositions() error {
	if om.positionManager == nil {
		return fmt.Errorf("position manager not initialized")
	}

	positions := om.positionManager.GetAllPositions()
	for symbol, pos := range positions {
		if pos.NetPos != 0 {
			side := models.SideSell
			if pos.NetPos < 0 {
				side = models.SideBuy
			}
			qty := abs(pos.NetPos)
			om.log.Infof("Flattening position for %s: %s %d", symbol, side, qty)
			if _, err := om.SubmitMarketOrder(symbol, side, qty); err != nil {
				om.log.Errorf("Failed to flatten position for %s: %v", symbol, err)
			}
		}
	}
	return nil
}

// GetRiskManager returns the risk manager
func (om *OrderManager) GetRiskManager() *risk.RiskManager {
	return om.riskManager
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
