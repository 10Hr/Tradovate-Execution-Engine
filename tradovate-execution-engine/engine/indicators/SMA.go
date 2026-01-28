package indicators

import "sync"

// UpdateMode defines when the SMA should update
type UpdateMode int

const (
	OnEachTick UpdateMode = iota
	OnBarClose
)

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

// DataSeriesHelper provides a way to access the circular buffer using LIFO indexing
type DataSeriesHelper struct {
	sma *SMA
}

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

// NewSMA creates a new SMA indicator with circular buffers
func NewSMA(period int, mode UpdateMode) *SMA {
	s := &SMA{
		period:     period,
		updateMode: mode,
		prices:     make([]float64, period),
		values:     make([]float64, period*2), // 2x period for safe lookback
	}
	s.Value = DataSeriesHelper{sma: s}
	return s
}

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

// CurrentValue returns the most recent SMA value
func (s *SMA) CurrentValue() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastValue
}

// Reset clears the buffers
func (s *SMA) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.priceIdx = 0
	s.priceCount = 0
	s.runningSum = 0
	s.valueIdx = 0
	s.valueCount = 0
	s.lastValue = 0
}
