package strategies

import (
	"fmt"
	"strconv"
	"tradovate-execution-engine/engine/indicators"
	"tradovate-execution-engine/engine/internal/execution"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/models"
)

// Position represents the current position
type Position int

const (
	Flat Position = iota
	Long
	Short
)

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

// NewDefaultMACrossover creates a new MA crossover strategy used for testing
func NewMACrossover(symbol string, fast, slow int, mode indicators.UpdateMode) *MACrossover {
	return &MACrossover{
		symbol:     symbol,
		fastLength: fast,
		slowLength: slow,
		mode:       mode,
		position:   Flat,
		enabled:    false,
	}
}

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

// Name returns the strategy name
func (m *MACrossover) Name() string {
	return "MA Crossover"
}

// Description returns the strategy description
func (m *MACrossover) Description() string {
	return "Moving Average Crossover strategy - generates buy signals when fast MA crosses above slow MA, and sell signals when fast MA crosses below slow MA"
}

// GetParams returns the configurable parameters
func (m *MACrossover) GetParams() []execution.StrategyParam {
	return []execution.StrategyParam{
		{
			Name:        "symbol",
			Type:        "string",
			Value:       m.symbol,
			Description: "Trading symbol",
		},
		{
			Name:        "fast_length",
			Type:        "int",
			Value:       strconv.Itoa(m.fastLength),
			Description: "Fast SMA period length",
		},
		{
			Name:        "slow_length",
			Type:        "int",
			Value:       strconv.Itoa(m.slowLength),
			Description: "Slow SMA period length",
		},
		{
			Name:        "update_mode",
			Type:        "string",
			Value:       strconv.Itoa(int(m.mode)),
			Description: "Update mode: 0=OnEachTick, 1=OnBarClose",
		},
	}
}

// SetParam sets a parameter value
func (m *MACrossover) SetParam(name, value string) error {
	if m.initialized {
		return fmt.Errorf("cannot modify parameters after initialization")
	}

	switch name {
	case "symbol":
		m.symbol = value
	case "fast_length":
		val, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid fast_length: %w", err)
		}
		if val <= 0 {
			return fmt.Errorf("fast_length must be positive")
		}
		m.fastLength = val
	case "slow_length":
		val, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid slow_length: %w", err)
		}
		if val <= 0 {
			return fmt.Errorf("slow_length must be positive")
		}
		m.slowLength = val
	case "update_mode":
		val, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid update_mode: %w", err)
		}
		m.mode = indicators.UpdateMode(val)
	default:
		return fmt.Errorf("unknown parameter: %s", name)
	}
	return nil
}

// SetEnabled enables or disables trading actions
func (m *MACrossover) SetEnabled(enabled bool) {
	m.enabled = enabled
}

// Init initializes the strategy with the order manager
func (m *MACrossover) Init(om *execution.OrderManager) error {
	if m.initialized {
		return fmt.Errorf("strategy already initialized")
	}

	if m.fastLength >= m.slowLength {
		return fmt.Errorf("fast_length (%d) must be less than slow_length (%d)", m.fastLength, m.slowLength)
	}

	m.orderMgr = om
	m.fastSMA = indicators.NewSMA(m.fastLength, m.mode)
	m.slowSMA = indicators.NewSMA(m.slowLength, m.mode)
	m.position = Flat
	m.initialized = true

	return nil
}

// OnBar processes a completed bar (for OnBarClose mode)
func (m *MACrossover) OnBar(timestamp string, price float64) error {
	if !m.initialized {
		return fmt.Errorf("strategy not initialized")
	}

	// Skip if we already processed this bar
	if timestamp == m.lastBarTimestamp {
		return nil
	}
	m.lastBarTimestamp = timestamp

	// Update SMAs with bar close price
	m.fastSMA.Update(price)
	m.slowSMA.Update(price)

	// Check for crossover signal
	newPosition, changed := m.checkSignal(1)
	if !changed {
		return nil
	}

	if m.logger != nil {
		if m.enabled {
			m.logger.Infof("! Signal detected at bar %s | Fast: %.2f | Slow: %.2f | New Position: %v !",
				timestamp, m.fastSMA.CurrentValue(), m.slowSMA.CurrentValue(), newPosition)
		}

	}

	return m.executePositionChange(newPosition)
}

// executePositionChange handles position transitions
func (m *MACrossover) executePositionChange(newPosition Position) error {

	if !m.enabled {
		if m.logger != nil {
			m.logger.Debug("[Disabled] ")
		}
		return nil
	}
	var side models.OrderSide
	var quantity int
	var logMsg string

	switch {
	case m.position == Flat && newPosition == Long:
		logMsg = "GOING LONG"
		side = models.SideBuy
		quantity = 1

	case m.position == Flat && newPosition == Short:
		logMsg = "GOING SHORT"
		side = models.SideSell
		quantity = 1

	case m.position == Long && newPosition == Short:
		logMsg = "REVERSING: Long → Flat"
		side = models.SideSell
		quantity = 1

	case m.position == Short && newPosition == Long:
		logMsg = "REVERSING: Short → Flat"
		side = models.SideBuy
		quantity = 1

	default:
		return nil
	}

	m.position = newPosition

	if m.logger != nil {
		m.logger.Info(logMsg)
	}

	_, err := m.orderMgr.SubmitMarketOrder(m.symbol, side, quantity)
	return err
}

// checkSignal checks for crossover signals
func (m *MACrossover) checkSignal(lookback int) (Position, bool) {
	if m.CrossAbove(lookback) {
		if m.position == Flat {
			return Long, true
		}
		if m.position == Short {
			return Long, true
		}
	} else if m.CrossBelow(lookback) {
		if m.position == Flat {
			return Short, true
		}
		if m.position == Long {
			return Short, true
		}
	}

	return m.position, false
}

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

// CrossBelow checks if fast MA crossed below slow MA within the last x bars
func (m *MACrossover) CrossBelow(barsAgo int) bool {
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

	return fastPrev >= slowPrev && fastNow < slowNow
}

// GetPosition returns the current position
func (m *MACrossover) GetPosition() Position {
	return m.position
}

// GetMetrics returns real-time metrics for the strategy
func (m *MACrossover) GetMetrics() map[string]float64 {
	metrics := make(map[string]float64)
	if m.fastSMA != nil {
		metrics["Fast SMA"] = m.fastSMA.Value.Get(0)
	}
	if m.slowSMA != nil {
		metrics["Slow SMA"] = m.slowSMA.Value.Get(0)
	}
	return metrics
}

// Reset resets the strategy state
func (m *MACrossover) Reset() {
	if m.fastSMA != nil {
		m.fastSMA.Reset()
	}
	if m.slowSMA != nil {
		m.slowSMA.Reset()
	}
	m.position = Flat
	m.lastBarTimestamp = ""
	m.initialized = false
}

// Register the strategy with the registry
func init() {
	execution.Register("ma_crossover", func(l *logger.Logger) execution.Strategy {
		return NewDefaultMACrossover(l)
	})
}
