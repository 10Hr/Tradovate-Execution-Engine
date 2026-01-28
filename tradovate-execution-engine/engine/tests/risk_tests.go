package tests

import (
	"fmt"
	"tradovate-execution-engine/engine/config"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/models"
	"tradovate-execution-engine/engine/internal/portfolio"
	"tradovate-execution-engine/engine/internal/risk"
)

// RunRiskTests executes all tests for the RiskManager.
func RunRiskTests() {
	testMaxContractsLimit()
	testDailyLossLimit()
	testIsDailyLossExceeded()
}

func testMaxContractsLimit() {
	log := logger.NewLogger(10)
	cfg := &config.Config{
		Risk: config.RiskConfig{
			MaxContracts:     2,
			DailyLossLimit:   10000, // High limit so it doesn't interfere
			EnableRiskChecks: true,
		},
	}
	rm := risk.NewRiskManager(cfg, log)

	pos := &portfolio.PLEntry{NetPos: 0}
	order := &models.Order{Side: models.SideBuy, Quantity: 1}

	// 1. Valid order
	err := rm.CheckOrderRisk(order, pos)
	if err != nil {
		check(fmt.Sprintf("Risk check should pass for order within limits (Error: %v)", err), false)
	} else {
		check("Risk check should pass for order within limits", true)
	}

	// 2. Order that exceeds limit
	order.Quantity = 3
	err = rm.CheckOrderRisk(order, pos)
	check("Risk check should fail for order exceeding max contracts", err != nil)

	// 3. Combined with current position
	order.Quantity = 2
	pos.NetPos = 1 // Already Long 1. New order for 2 would make it 3.
	err = rm.CheckOrderRisk(order, pos)
	check("Risk check should fail when position + new order > max", err != nil)

	// 4. Sell order that exceeds short limit
	order.Side = models.SideSell
	order.Quantity = 4
	pos.NetPos = 0
	err = rm.CheckOrderRisk(order, pos) // Would make it -4
	check("Risk check should fail for short order exceeding max contracts", err != nil)
}

func testDailyLossLimit() {
	log := logger.NewLogger(10)
	cfg := &config.Config{
		Risk: config.RiskConfig{
			MaxContracts:     10, // High limit so it doesn't interfere
			DailyLossLimit:   500,
			EnableRiskChecks: true,
		},
	}
	rm := risk.NewRiskManager(cfg, log)

	pos := &portfolio.PLEntry{NetPos: 0}
	order := &models.Order{Side: models.SideBuy, Quantity: 1}

	// 1. Under loss limit
	rm.UpdatePnL(-400)
	err := rm.CheckOrderRisk(order, pos)
	if err != nil {
		check(fmt.Sprintf("Risk check should pass when under daily loss limit (Error: %v)", err), false)
	} else {
		check("Risk check should pass when under daily loss limit", true)
	}

	// 2. Over loss limit
	rm.UpdatePnL(-200) // Total PnL: -600
	err = rm.CheckOrderRisk(order, pos)
	check("Risk check should fail when over daily loss limit", err != nil)
}

func testIsDailyLossExceeded() {
	log := logger.NewLogger(10)
	cfg := &config.Config{
		Risk: config.RiskConfig{
			DailyLossLimit: 500,
		},
	}
	rm := risk.NewRiskManager(cfg, log)

	check("Daily loss not exceeded at $0", !rm.IsDailyLossExceeded(0))
	check("Daily loss not exceeded at -$499", !rm.IsDailyLossExceeded(-499))
	check("Daily loss exceeded at -$500", rm.IsDailyLossExceeded(-500))
	check("Daily loss exceeded at -$1000", rm.IsDailyLossExceeded(-1000))
}
