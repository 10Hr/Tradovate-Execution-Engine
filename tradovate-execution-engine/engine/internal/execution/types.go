package execution

import (
	"sync"
	"tradovate-execution-engine/engine/config"
	"tradovate-execution-engine/engine/internal/auth"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/models"
	"tradovate-execution-engine/engine/internal/portfolio"
	"tradovate-execution-engine/engine/internal/risk"
)

//
// ORDER MANAGER
//

// OrderManager handles order submission and tracking
type OrderManager struct {
	Mu               sync.RWMutex
	orders           map[string]*models.Order // Map of order ID to order
	tokenManager     *auth.TokenManager
	portfolioTracker *portfolio.PortfolioTracker
	riskManager      *risk.RiskManager
	config           *config.Config
	log              *logger.Logger
	orderIDCounter   int
}

//
// STRATEGY MANAGER
//

// StrategyParam represents a single configuration parameter for a strategy
type StrategyParam struct {
	Name        string
	Type        string // "int", "float", "string"
	Value       string
	Description string
}

// Strategy interface defines the required methods for any trading strategy
type Strategy interface {
	Name() string
	Description() string
	GetParams() []StrategyParam
	SetParam(name, value string) error
	Init(om *OrderManager) error
	GetMetrics() map[string]float64
	Reset()
}

// StrategyRegistry maintains a list of available strategies
type StrategyRegistry struct {
	mu         sync.RWMutex
	strategies map[string]func(*logger.Logger) Strategy
}

var globalRegistry = &StrategyRegistry{
	strategies: make(map[string]func(*logger.Logger) Strategy),
}
