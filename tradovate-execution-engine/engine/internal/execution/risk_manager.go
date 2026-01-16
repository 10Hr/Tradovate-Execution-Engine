package execution

import (
	"fmt"
	"sync"
	"time"

	"tradovate-execution-engine/engine/internal/logger"
)

// RiskManager handles risk checks and limits
type RiskManager struct {
	mu            sync.RWMutex
	config        *Config
	dailyPnL      float64
	dailyPnLReset time.Time
	tradeCount    int
	log           *logger.Logger
}

// NewRiskManager creates a new risk manager
func NewRiskManager(config *Config, log *logger.Logger) *RiskManager {
	return &RiskManager{
		config:        config,
		dailyPnL:      0,
		dailyPnLReset: time.Now(),
		tradeCount:    0,
		log:           log,
	}
}

// CheckOrderRisk validates if an order passes risk checks
func (rm *RiskManager) CheckOrderRisk(order *Order, currentPosition *Position) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if !rm.config.EnableRiskChecks {
		return nil
	}

	// Reset daily PnL if it's a new day
	if time.Since(rm.dailyPnLReset) > 24*time.Hour {
		rm.resetDailyPnL()
	}

	// Check daily loss limit
	if rm.dailyPnL <= -rm.config.DailyLossLimit {
		rm.log.Error("Daily loss limit reached")
		return fmt.Errorf("daily loss limit of $%.2f reached (current: $%.2f)",
			rm.config.DailyLossLimit, rm.dailyPnL)
	}

	// Check max contracts
	newPositionSize := rm.calculateNewPositionSize(order, currentPosition)
	if abs(newPositionSize) > rm.config.MaxContracts {
		rm.log.Errorf("Order would exceed max contracts limit: %d", rm.config.MaxContracts)
		return fmt.Errorf("order would exceed max contracts limit of %d", rm.config.MaxContracts)
	}

	rm.log.Infof("Risk check passed for order: %s %d %s", order.Side, order.Quantity, order.Symbol)
	return nil
}

// UpdatePnL updates the daily PnL
func (rm *RiskManager) UpdatePnL(pnl float64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.dailyPnL += pnl
	rm.log.Infof("Daily PnL updated: $%.2f (change: $%.2f)", rm.dailyPnL, pnl)
}

// GetDailyPnL returns the current daily PnL
func (rm *RiskManager) GetDailyPnL() float64 {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.dailyPnL
}

// IncrementTradeCount increments the daily trade counter
func (rm *RiskManager) IncrementTradeCount() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.tradeCount++
	rm.log.Infof("Trade count: %d", rm.tradeCount)
}

// GetTradeCount returns the daily trade count
func (rm *RiskManager) GetTradeCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.tradeCount
}

// resetDailyPnL resets daily statistics (called at day start)
func (rm *RiskManager) resetDailyPnL() {
	rm.dailyPnL = 0
	rm.tradeCount = 0
	rm.dailyPnLReset = time.Now()
	rm.log.Info("Daily PnL and trade count reset")
}

// calculateNewPositionSize calculates what the position size would be after the order
func (rm *RiskManager) calculateNewPositionSize(order *Order, currentPosition *Position) int {
	currentQty := 0
	if currentPosition != nil {
		currentQty = currentPosition.Quantity
	}

	orderQty := order.Quantity
	if order.Side == SideSell {
		orderQty = -orderQty
	}

	return currentQty + orderQty
}

// abs returns absolute value
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
