package execution

import (
	"sync"
	"tradovate-execution-engine/engine/config"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/models"
	"tradovate-execution-engine/engine/internal/portfolio"
	"tradovate-execution-engine/engine/internal/risk"
)

// OrderManager handles order submission and tracking
type OrderManager struct {
	Mu              sync.RWMutex
	orders          map[string]*models.Order // Map of order ID to order
	positionManager *portfolio.PositionManager
	riskManager     *risk.RiskManager
	config          *config.Config
	log             *logger.Logger
	orderIDCounter  int
}
