package execution

// Config holds risk management and order configuration
type Config struct {
	MaxContracts     int     // Maximum contracts per position
	DailyLossLimit   float64 // Daily loss limit in dollars
	MaxOrderRetries  int     // Maximum number of order submission retries
	OrderTimeout     int     // Order timeout in seconds
	EnableRiskChecks bool    // Enable/disable risk checks
}

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
