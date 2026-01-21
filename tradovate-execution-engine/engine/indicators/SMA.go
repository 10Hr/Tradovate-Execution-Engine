package indicators

// UpdateMode defines when the SMA should update
type UpdateMode int

const (
	OnEachTick UpdateMode = iota
	OnBarClose
)

// DataSeries is a LIFO-indexed slice where [0] is most recent
type DataSeries []float64

// Get returns value with LIFO indexing: [0] = current, [1] = 1 bar ago, etc.
func (ds DataSeries) Get(index int) float64 {
	if index < 0 || len(ds) == 0 {
		return 0
	}

	actualIndex := len(ds) - 1 - index
	if actualIndex < 0 {
		return 0
	}

	return ds[actualIndex]
}

// SMA represents a Simple Moving Average indicator
type SMA struct {
	period     int
	prices     []float64 // Rolling window of input prices
	updateMode UpdateMode
	Value      DataSeries // Historical SMA values with LIFO indexing
}

// NewSMA creates a new SMA indicator with the specified period and update mode
func NewSMA(period int, mode UpdateMode) *SMA {
	return &SMA{
		period:     period,
		prices:     make([]float64, 0, period),
		updateMode: mode,
		Value:      make(DataSeries, 0),
	}
}

// Update adds a new bar's closing price and calculates the SMA
func (s *SMA) Update(price float64) float64 {
	s.prices = append(s.prices, price)

	// Keep only the last 'period' prices
	if len(s.prices) > s.period {
		s.prices = s.prices[1:]
	}

	smaValue := s.calculate()

	// Store the calculated SMA value
	s.Value = append(s.Value, smaValue)

	return smaValue
}

// calculate computes the current SMA
func (s *SMA) calculate() float64 {
	if len(s.prices) < s.period {
		return 0 // Not enough data yet
	}

	sum := 0.0
	for _, price := range s.prices {
		sum += price
	}

	return sum / float64(s.period)
}

// CurrentValue returns the most recent SMA value (same as Value[0])
func (s *SMA) CurrentValue() float64 {
	return s.Value.Get(0)
}

// Reset clears all values
func (s *SMA) Reset() {
	s.prices = s.prices[:0]
	s.Value = s.Value[:0]
}

// package indicators

// // SMA represents a Simple Moving Average indicator
// type SMA struct {
// 	period int
// 	values []float64
// }

// // NewSMA creates a new SMA indicator with the specified period
// func NewSMA(period int) *SMA {
// 	return &SMA{
// 		period: period,
// 		values: make([]float64, 0, period),
// 	}
// }

// // Update adds a new value and returns the current SMA
// // Returns 0 if there aren't enough values yet
// func (s *SMA) Update(value float64) float64 {
// 	s.values = append(s.values, value)

// 	// Keep only the last 'period' values
// 	if len(s.values) > s.period {
// 		s.values = s.values[1:]
// 	}

// 	// Calculate SMA
// 	if len(s.values) < s.period {
// 		return 0 // Not enough data yet
// 	}

// 	sum := 0.0
// 	for _, v := range s.values {
// 		sum += v
// 	}

// 	return sum / float64(s.period)
// }

// // Value returns the current SMA without updating
// func (s *SMA) Value() float64 {
// 	if len(s.values) < s.period {
// 		return 0
// 	}

// 	sum := 0.0
// 	for _, v := range s.values {
// 		sum += v
// 	}

// 	return sum / float64(s.period)
// }

// // Reset clears all values
// func (s *SMA) Reset() {
// 	s.values = s.values[:0]
// }
