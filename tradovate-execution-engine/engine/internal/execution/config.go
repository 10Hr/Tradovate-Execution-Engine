package execution

import "tradovate-execution-engine/engine/config"

// Config holds risk management and order configuration
type Config = config.RiskConfig

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		MaxContracts:     1,
		DailyLossLimit:   500.0,
		MaxOrderRetries:  3,
		OrderTimeout:     30,
		EnableRiskChecks: true,
	}
}

// NewConfig creates a new config with custom values
func NewConfig(maxContracts int, dailyLossLimit float64) *Config {
	return &Config{
		MaxContracts:     maxContracts,
		DailyLossLimit:   dailyLossLimit,
		MaxOrderRetries:  3,
		OrderTimeout:     30,
		EnableRiskChecks: true,
	}
}
