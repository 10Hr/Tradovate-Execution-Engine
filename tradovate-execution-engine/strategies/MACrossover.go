package strategies

import "tradovate-execution-engine/indicators"

// Position represents the current position
type Position int

const (
	Flat Position = iota
	Long
	Short
)

// MACrossover implements a moving average crossover strategy
type MACrossover struct {
	fastSMA  *indicators.SMA
	slowSMA  *indicators.SMA
	position Position
}

// NewMACrossover creates a new MA crossover strategy with 5 and 15 period SMAs
func NewMACrossover(mode indicators.UpdateMode) *MACrossover {
	return &MACrossover{
		fastSMA:  indicators.NewSMA(5, mode),
		slowSMA:  indicators.NewSMA(15, mode),
		position: Flat,
	}
}

// Update processes a new price and returns the signal
// Returns: new position, changed (true if position changed)
func (m *MACrossover) Update(price float64) (Position, bool) {
	m.fastSMA.Update(price)
	m.slowSMA.Update(price)

	return m.checkSignal(1) // Check for crossover in the last 1 bar
}

// checkSignal checks for crossover signals
func (m *MACrossover) checkSignal(lookback int) (Position, bool) {
	if m.CrossAbove(lookback) {
		if m.position != Long {
			m.position = Long
			return Long, true
		}
	} else if m.CrossBelow(lookback) {
		if m.position != Short {
			m.position = Short
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

	// Fast was below or equal, now above
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

	// Fast was above or equal, now below
	return fastPrev >= slowPrev && fastNow < slowNow
}

// GetPosition returns the current position
func (m *MACrossover) GetPosition() Position {
	return m.position
}

// Reset resets the strategy state
func (m *MACrossover) Reset() {
	m.fastSMA.Reset()
	m.slowSMA.Reset()
	m.position = Flat
}
