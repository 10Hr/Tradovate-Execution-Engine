# Tradovate Execution Engine - Architecture Documentation

## Overview

The Tradovate Execution Engine is a terminal-based automated trading system built in Go. This document provides technical architecture details with code citations from the actual implementation.

## Table of Contents
1. [System Architecture](#system-architecture)
2. [Core Components](#core-components)
3. [Data Structures](#data-structures)
4. [Key Design Decisions](#key-design-decisions)
5. [Concurrency Model](#concurrency-model)
6. [Known Limitations](#known-limitations)

---

## System Architecture

### Entry Point

**File:** `engine/cmd/main.go` (lines 12-21)

```go
func main() {

	tests.RunAllTests()

	p := tea.NewProgram(UI.InitialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}

}
```

The application:
1. Runs all automated tests first (line 14)
2. Initializes the Bubbletea TUI program (line 16)
3. Launches in alternate screen mode
4. Blocks on the UI event loop

### Configuration System

**File:** `engine/config/types.go` (lines 7-32)

The configuration is split into two main sections:

```go
// Config holds all configuration settings
type Config struct {
	Tradovate TradovateConfig `json:"tradovate"`
	Risk      RiskConfig      `json:"risk"`
}

// TradovateConfig holds Tradovate-specific credentials
type TradovateConfig struct {
	AppID       string `json:"appId"`
	AppVersion  string `json:"appVersion"`
	Chl         string `json:"chl"`
	Cid         string `json:"cid"`
	DeviceID    string `json:"deviceId"`
	Environment string `json:"environment"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Sec         string `json:"sec"`
	Enc         bool   `json:"enc"`
}

// RiskConfig holds risk management and order configuration
type RiskConfig struct {
	MaxContracts     int     `json:"maxContracts"`
	DailyLossLimit   float64 `json:"dailyLossLimit"`
	EnableRiskChecks bool    `json:"enableRiskChecks"`
}
```

**Config Loading:** `engine/config/config.go` (lines 52-79)

The system auto-creates a default config if none exists:

```go
func LoadOrCreateConfig(logger *logger.Logger) (*Config, error) {
	configPath := GetConfigPath()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Config doesn't exist, create default
		logger.Warnf("Config file not found at %s. Creating default config...", configPath)
		if err := CreateDefaultConfig(configPath); err != nil {
			return nil, fmt.Errorf("Failed to create default config: %w", err)
		}
		logger.Infof("Default config created at %s", configPath)
		logger.Info("Please edit the config file with your credentials and retry.")
		return nil, fmt.Errorf("Default config created, please configure and retry")
	}
	// ... loads existing config
}
```

---

## Core Components

### 1. Strategy System

**Interface Definition:** `engine/internal/execution/types.go` (lines 41-50)

```go
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
```

**Strategy Registration:** `engine/internal/execution/strategy_manager.go` (lines 8-13)

```go
// Register adds a strategy to the global registry
func Register(name string, factory func(*logger.Logger) Strategy) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.strategies[name] = factory
}
```

**MA Crossover Registration:** `engine/strategies/MACrossover.go` (lines 334-339)

```go
// Register the strategy with the registry
func init() {
	execution.Register("ma_crossover", func(l *logger.Logger) execution.Strategy {
		return NewDefaultMACrossover(l)
	})
}
```

**Design:** Strategies self-register using Go's `init()` function, which runs automatically when the package is imported (see `cmd/main.go` line 6: `_ "tradovate-execution-engine/engine/strategies"`).

### 2. SMA Indicator

**File:** `engine/indicators/SMA.go`

**Data Structure** (lines 13-35):

```go
// SMA represents a Simple Moving Average indicator using a circular ring buffer
type SMA struct {
	mu         sync.RWMutex
	period     int
	updateMode UpdateMode

	// Circular buffer for input prices
	prices     []float64
	priceIdx   int
	priceCount int
	runningSum float64

	// Store last calculated value directly (avoids ring buffer retrieval issues)
	lastValue float64

	// Circular buffer for calculated SMA results (Size = Period * 2 for lookback)
	values     []float64
	valueIdx   int
	valueCount int

	// Value provides LIFO-like access for strategy logic
	Value DataSeriesHelper
}
```

**O(1) Update Algorithm** (lines 78-110):

```go
// Update adds a new price and returns the current SMA in O(1) time
func (s *SMA) Update(price float64) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 1. Update running sum (subtract oldest, add newest)
	if s.priceCount == s.period {
		s.runningSum -= s.prices[s.priceIdx]
	} else {
		s.priceCount++
	}

	s.prices[s.priceIdx] = price
	s.runningSum += price

	// Move price index forward
	s.priceIdx = (s.priceIdx + 1) % s.period

	// 2. Calculate SMA
	var smaValue float64
	if s.priceCount == s.period {
		smaValue = s.runningSum / float64(s.period)
	}

	// 3. Store the value
	s.lastValue = smaValue          // Store directly for fast access
	s.values[s.valueIdx] = smaValue // Also store in ring buffer for history
	s.valueIdx = (s.valueIdx + 1) % len(s.values)
	if s.valueCount < len(s.values) {
		s.valueCount++
	}

	return smaValue
}
```

**LIFO Historical Access** (lines 42-64):

```go
// Get returns historical SMA values: [0] = current, [1] = 1 back, etc.
func (h DataSeriesHelper) Get(index int) float64 {
	if h.sma == nil {
		return 0
	}

	s := h.sma
	s.mu.RLock()
	defer s.mu.RUnlock()

	if index < 0 || index >= s.valueCount {
		return 0
	}

	if index == 0 {
		return s.lastValue
	}

	size := len(s.values)
	targetIdx := (s.valueIdx - 1 - index + size) % size

	return s.values[targetIdx]
}
```

**Design Note:** The LIFO indexing (`Value.Get(0)` = current, `Value.Get(1)` = previous) was designed to match NinjaTrader's DataSeries pattern for familiarity.

### 3. Risk Management

**File:** `engine/internal/risk/risk_manager.go`

**Data Structure:** `types.go` (lines 11-19)

```go
// RiskManager handles risk checks and limits
type RiskManager struct {
	mu            sync.RWMutex
	config        *config.Config
	dailyPnL      float64
	dailyPnLReset time.Time
	tradeCount    int
	log           *logger.Logger
}
```

**Pre-Order Risk Checks** (lines 24-72):

```go
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

	rm.log.Infof("Risk check passed for order: %s %d %s", order.Side, order.Quantity, order.Symbol)
	return nil
}
```

**Real-Time Loss Limit Check** (lines 74-79):

```go
// IsDailyLossExceeded checks if the daily loss limit has been met
func (rm *RiskManager) IsDailyLossExceeded(currentTotalPnL float64) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return currentTotalPnL <= -rm.config.Risk.DailyLossLimit
}
```

This is called every second from the UI update loop to monitor P&L in real-time.

### 4. Order Management

**File:** `engine/internal/execution/types.go` (lines 17-27)

```go
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
```

**Order Model:** `engine/internal/models/order.go` (lines 36-48)

```go
// Order represents a trading order
type Order struct {
	ID           string      // Unique order identifier
	Symbol       string      // Trading symbol
	Side         OrderSide   // Buy or Sell
	Type         OrderType   // Market, Limit, Stop
	Quantity     int         // Number of contracts
	Price        float64     // Order price (0 for market orders)
	Status       OrderStatus // Current order status
	SubmittedAt  time.Time   // When order was submitted
	RejectReason string      // Reason for rejection if applicable
	ExternalID   string      // External order ID from broker
}
```

**Order Status:** `engine/internal/models/order.go` (lines 10-17)

```go
const (
	StatusPending   OrderStatus = "PENDING"
	StatusSubmitted OrderStatus = "SUBMITTED"
	StatusFilled    OrderStatus = "FILLED"
	StatusRejected  OrderStatus = "REJECTED"
	StatusCanceled  OrderStatus = "CANCELED"
	StatusFailed    OrderStatus = "FAILED"
)
```

### 5. MA Crossover Strategy

**File:** `engine/strategies/MACrossover.go`

**Data Structure** (lines 21-37):

```go
// MACrossover implements a moving average crossover strategy
type MACrossover struct {
	symbol      string
	fastSMA     *indicators.SMA
	slowSMA     *indicators.SMA
	position    Position
	fastLength  int
	slowLength  int
	mode        indicators.UpdateMode
	orderMgr    *execution.OrderManager
	logger      *logger.Logger
	initialized bool

	// Track last bar timestamp to avoid processing same bar multiple times
	lastBarTimestamp string
	enabled          bool
}
```

**Default Parameters** (lines 51-62):

```go
// NewDefaultMACrossover creates a new MA crossover strategy with default settings
func NewDefaultMACrossover(l *logger.Logger) *MACrossover {
	return &MACrossover{
		symbol:     "MESH6",
		fastLength: 5,
		slowLength: 15,
		mode:       indicators.OnBarClose,
		position:   Flat,
		enabled:    false,
		logger:     l,
	}
}
```

**Crossover Detection** (lines 266-283):

```go
// CrossAbove checks if fast MA crossed above slow MA within the last x bars
func (m *MACrossover) CrossAbove(barsAgo int) bool {
	fastNow := m.fastSMA.Value.Get(0)
	slowNow := m.slowSMA.Value.Get(0)

	if fastNow == 0 || slowNow == 0 {
		return false
	}

	fastPrev := m.fastSMA.Value.Get(barsAgo)
	slowPrev := m.slowSMA.Value.Get(barsAgo)

	if fastPrev == 0 || slowPrev == 0 {
		return false
	}

	return fastPrev <= slowPrev && fastNow > slowNow
}
```

**Signal Logic:**
- `CrossAbove`: Fast SMA was below/equal slow SMA previously, now above (bullish signal)
- `CrossBelow`: Fast SMA was above/equal slow SMA previously, now below (bearish signal)

---

## Data Structures

### Logger

**File:** `engine/internal/logger/types.go` (lines 15-27)

```go
// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time
	Level     LogLevel
	Message   string
}

// Logger handles all logging throughout the application
type Logger struct {
	mu      sync.RWMutex
	entries []LogEntry
	maxSize int
}
```

The logger uses a circular buffer with a maximum of 500 entries (configurable via `maxSize`).

### Token Manager

**File:** `engine/internal/auth/types.go` (lines 14-28)

```go
// TokenManager manages authentication tokens for Tradovate API
type TokenManager struct {
	mu              sync.RWMutex
	accessToken     string
	mdAccessToken   string
	expirationTime  time.Time
	userID          int
	accountID       int
	username        string
	credentials     map[string]interface{}
	baseURL         string
	log             *logger.Logger
	config          config.Config
	monitorStopChan chan struct{}
}
```

Stores both trading and market data access tokens, manages expiration, and tracks user/account information.

---

## Key Design Decisions

### 1. Why Circular Buffer for SMA?

**Problem:** Traditional slice-based SMA calculation is O(n) per update:
- Must sum all n values on each update
- May require memory reallocation as slices grow

**Solution:** Circular buffer with running sum achieves O(1):
- Maintains a running sum
- Subtract oldest value, add newest value
- Fixed memory allocation

**Code:** `indicators/SMA.go` lines 82-93

### 2. Why Strategy Registry Pattern?

**Design:** Strategies self-register via `init()` functions.

**Benefits:**
- Auto-discovery: No manual registration needed
- Loose coupling: New strategies added without modifying core code
- Type safety: Factory pattern ensures correct instantiation

**Code:** `strategies/MACrossover.go` lines 334-339

### 3. Why Two-Layer Risk System?

**Layer 1:** Pre-order checks (preventive)
- Run before order submission
- Block orders that would violate limits

**Layer 2:** Real-time monitoring (reactive)
- Runs every second via UI tick
- Auto-flattens on limit breach
- Auto-stops strategy

**Rationale:** Defense in depth - prevents violations before they occur, catches violations that slip through.

### 4. Why Use Tradovate's API P&L for Risk Checks?

**Decision:** Trust Tradovate's reported P&L for risk decisions.

**Rationale:**
- Includes all fees (exchange, clearing, regulatory)
- Includes commissions
- Accounts for slippage
- Single source of truth
- Prevents calculation drift

**Note:** Internal P&L still tracked for display and learning purposes.

**Code:** Risk checks use `currentTotalPnL` from API in `UIPanel.go`

### 5. Why A TUI?

**Chosen Framework:** Charm Bracelet's Bubbletea

**Rationale:**

**Performance**
- Minimal overhead
- Lightweight footprint leaves resources for the core engine

**Operational Benefits**
- Keyboard driven workflows
- Natural fit for headless deployments

**Development Advantages**
- Easy to add new commands as the engine evolves
- Rich Charm ecosystem (Lipgloss, Bubbles)
- Cross-platform: Linux, macOS, Windows

**Integration**
- Scriptable and pipeable
- Works with existing terminal tooling
- Simple to automate

### 6. Why Tests Run on Startup?

**Current:** `cmd/main.go` line 14 runs tests before UI

**Rationale:**
- Validates core functionality before trading
- Quick execution (~1-2 seconds)
- Demonstrates test coverage

**Production Note:** Can comment out for faster startup if needed.

---

## Concurrency Model

### Goroutines

**Active goroutines during operation:**

1. Main goroutine: Bubbletea event loop
2. Market Data WebSocket: Message reader
3. Market Data WebSocket: Heartbeat sender
4. Trading WebSocket: Message reader  
5. Trading WebSocket: Heartbeat sender

**Total: ~5 goroutines** (lightweight)

### Thread Safety

**Mutexes:**

All shared data structures use mutexes:

```go
// From execution/types.go
type OrderManager struct {
	Mu sync.RWMutex  // Protects orders map
	// ...
}

// From risk/types.go
type RiskManager struct {
	mu sync.RWMutex  // Protects PnL data
	// ...
}

// From indicators/SMA.go
type SMA struct {
	mu sync.RWMutex  // Protects buffers
	// ...
}
```

**Read-Write Locks (`sync.RWMutex`):**
- Allow multiple concurrent readers
- Exclusive lock for writers
- Optimizes for read-heavy workloads

### No Channels for Data Flow

**Design:** The system uses mutex-protected shared state rather than channels.

**Rationale:**
- Simpler mental model for this use case
- Direct data access (no channel send/receive overhead)
- Works well with Bubbletea's pull-based update model

---

## Known Limitations

### By Design (Scope)

1. **Single Strategy:** Only MA Crossover implemented
2. **1-Minute Bars Only:** No multi-timeframe or tick analysis
3. **Fixed Position Sizing:** 1 contract per trade (hardcoded in strategy)
4. **No Partial Fill Handling:** Assumes complete fills
5. **Manual Reconnection:** No automatic reconnection on WebSocket disconnect
6. **No API Key:** Uses browser-based credential extraction

### Technical

1. **State Synchronization:**
   - Engine maintains internal position state
   - Does not sync with external Tradovate platform changes
   - ⚠️ I would not recommend you trade manually on Tradovate while engine is running

2. **Session Realized P&L:**
   - Does not account for fees or commissions
   - Shows gross P&L only
   - Refer to Tradovate account statement for net P&L

   Note: This is only in engine

3. **Data Persistence:**
   - No database
   - Logs limited to 500 (configurable in code) entries in memory
   - Must export logs before closing

4. **Order Types:**
   - Market orders only
   - No limit, stop, or bracket orders

5. **Testing:**
   - Tests run on every startup
   - No flag to skip (must comment out code)

### Future Enhancements

**Short Term:**
- Auto-reconnection with exponential backoff
- Additional strategies (Breakout, RSI)
- Partial fill handling

**Medium Term:**
- Database for trade history
- Multi-timeframe analysis
- Advanced order types

**Long Term:**
- Backtesting framework
- Machine learning integration
- Multi-asset support
- Full strategy script importer

---

## For More Information

**Setup Instructions:** See [SETUP.md](SETUP.md)  
**User Guide:** See [README.md](README.md)  
**Code Repository:** https://github.com/10Hr/Tradovate-Execution-Engine

---

**Document Version:** 1.0
**Based on commit:** 7f7e1d1
**Author:** Tyler (10Hr)
