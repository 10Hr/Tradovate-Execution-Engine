package portfolio

import (
	"sync"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/tradovate"
)

type PLEntry struct {
	Name      string
	PL        float64
	NetPos    int
	BuyPrice  float64
	LastPrice float64
}

// PLTracker manages profit & loss tracking
type PLTracker struct {
	entries map[string]*PLEntry
	mu      sync.RWMutex
	log     *logger.Logger

	realizedPnL        float64 // Today's closed trade P&L
	initialRealizedPnL float64 // P&L at start of session
	hasInitialRealized bool
}

// PortfolioTracker manages the entire portfolio tracking system
type PortfolioTracker struct {
	tradingSubsciptionManager *tradovate.DataSubscriber
	mdSubsciptionManager      *tradovate.DataSubscriber
	plTracker                 *PLTracker
	log                       *logger.Logger
	running                   bool
	mu                        sync.Mutex

	// State tracking
	userID    int
	positions map[int]*tradovate.APIPosition
	contracts map[int]string
	products  map[string]float64
}
