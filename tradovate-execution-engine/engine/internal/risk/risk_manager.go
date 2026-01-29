package risk

import (
	"fmt"
	"time"

	"tradovate-execution-engine/engine/config"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/models"
	"tradovate-execution-engine/engine/internal/portfolio"
)

// NewRiskManager creates a new risk manager
func NewRiskManager(config *config.Config, log *logger.Logger) *RiskManager {
	return &RiskManager{
		config:        config,
		dailyPnL:      0,
		dailyPnLReset: time.Now(),
		tradeCount:    0,
		log:           log,
	}
}

// CheckOrderRisk validates if an order passes risk checks
func (rm *RiskManager) CheckOrderRisk(order *models.Order, currentPosition *portfolio.PLEntry) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if !rm.config.Risk.EnableRiskChecks {
		return nil
	}

	// Reset daily PnL if it's a new day
	if time.Since(rm.dailyPnLReset) > 24*time.Hour {
		rm.resetDailyPnL()
	}

	// Check daily loss limit
	if rm.dailyPnL <= -rm.config.Risk.DailyLossLimit {
		rm.log.Error("Daily loss limit reached")
		return fmt.Errorf("daily loss limit of $%.2f reached (current: $%.2f)",
			rm.config.Risk.DailyLossLimit, rm.dailyPnL)
	}

	// Check max contracts
	currentQty := 0
	if currentPosition != nil {
		currentQty = currentPosition.NetPos
	}

	// Calculate potential position based on order side
	// We want to ensure that even if all working orders fill, we don't exceed limits
	if order.Side == models.SideBuy {
		// Current position + this new buy order
		potentialMaxLong := currentQty + order.Quantity
		if potentialMaxLong > rm.config.Risk.MaxContracts {
			rm.log.Errorf("Order would exceed max contracts limit: %d (Potential Long: %d)", rm.config.Risk.MaxContracts, potentialMaxLong)
			return fmt.Errorf("order would exceed max contracts limit of %d", rm.config.Risk.MaxContracts)
		}
	} else { // SideSell
		// Current position - this new sell order
		// Note: order.Quantity are positive, so we subtract them
		potentialMaxShort := currentQty - order.Quantity
		if potentialMaxShort < -rm.config.Risk.MaxContracts {
			rm.log.Errorf("Order would exceed max contracts limit: %d (Potential Short: %d)", rm.config.Risk.MaxContracts, potentialMaxShort)
			return fmt.Errorf("order would exceed max contracts limit of %d", rm.config.Risk.MaxContracts)
		}
	}

	rm.log.Debugf("Risk check passed for order: %s %d %s", order.Side, order.Quantity, order.Symbol)
	return nil
}

// IsDailyLossExceeded checks if the daily loss limit has been met
func (rm *RiskManager) IsDailyLossExceeded(currentTotalPnL float64) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return currentTotalPnL <= -rm.config.Risk.DailyLossLimit
}

// UpdatePnL updates the daily PnL
func (rm *RiskManager) UpdatePnL(pnl float64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.dailyPnL += pnl
	rm.log.Debugf("Daily PnL updated: $%.2f (change: $%.2f)", rm.dailyPnL, pnl)
}

// GetDailyPnL returns the current daily PnL
func (rm *RiskManager) GetDailyPnL() float64 {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.dailyPnL
}

// SetDailyPnL sets the daily PnL (e.g. from API sync)
func (rm *RiskManager) SetDailyPnL(pnl float64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.dailyPnL = pnl
}

// IncrementTradeCount increments the daily trade counter
func (rm *RiskManager) IncrementTradeCount() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.tradeCount++
	rm.log.Debugf("Trade count: %d", rm.tradeCount)
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
