package risk

import (
	"sync"
	"time"
	"tradovate-execution-engine/engine/config"

	"tradovate-execution-engine/engine/internal/logger"
)

// RiskManager handles risk checks and limits
type RiskManager struct {
	mu            sync.RWMutex
	config        *config.Config
	dailyPnL      float64
	dailyPnLReset time.Time
	tradeCount    int
	log           *logger.Logger
}
