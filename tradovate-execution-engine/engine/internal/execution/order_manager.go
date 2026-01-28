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

// SetPortfolioTracker sets the portfolio tracker for the order manager
func (om *OrderManager) SetPortfolioTracker(pt *portfolio.PortfolioTracker) {
	om.Mu.Lock()
	defer om.Mu.Unlock()
	om.portfolioTracker = pt
}

// SubmitMarketOrder submits a market order
func (om *OrderManager) Flatten(symbol string, side models.OrderSide, quantity int) (*models.Order, error) {
	om.Mu.Lock()

	// Generate order ID
	om.orderIDCounter++
	orderID := fmt.Sprintf("FLATTEN ORD-%s-%d-%d", symbol, time.Now().Unix(), om.orderIDCounter)

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

	// Submit order
	if err := om.submitOrderToExchange(order); err != nil {
		om.updateOrderStatus(orderID, models.StatusFailed, err.Error())
		return order, err
	}

	om.updateOrderStatus(orderID, models.StatusSubmitted, "")
	return order, nil
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
	var currentPosition *portfolio.PLEntry
	if om.portfolioTracker != nil {
		summary := om.portfolioTracker.GetPLSummary()
		if pos, ok := summary[symbol]; ok {
			currentPosition = &pos
		}
	}

	if err := om.riskManager.CheckOrderRisk(order, currentPosition); err != nil {
		om.updateOrderStatus(orderID, models.StatusRejected, err.Error())
		return order, fmt.Errorf("risk check failed: %w", err)
	}

	// Submit order
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
		order.ExternalID = fmt.Sprintf("%.0f", orderId)
	} else if orderIdStr, ok := result["orderId"].(string); ok {
		order.ExternalID = orderIdStr
	}

	om.log.Infof("Order %s submitted successfully (External ID: %s)", order.ID, order.ExternalID)
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
func (om *OrderManager) GetPosition(symbol string) portfolio.PLEntry {
	if om.portfolioTracker != nil {
		summary := om.portfolioTracker.GetPLSummary()
		if pos, ok := summary[symbol]; ok {
			return pos
		}
	}
	return portfolio.PLEntry{Name: symbol}
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
	if om.portfolioTracker == nil {
		return fmt.Errorf("portfolio tracker not initialized")
	}

	summary := om.portfolioTracker.GetPLSummary()

	for symbol, pos := range summary {
		if pos.NetPos != 0 {
			side := models.SideSell
			if pos.NetPos < 0 {
				side = models.SideBuy
			}
			qty := models.Abs(pos.NetPos)
			om.log.Infof("Flattening position for %s: %s %d", symbol, side, qty)
			if _, err := om.Flatten(symbol, side, qty); err != nil {
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
