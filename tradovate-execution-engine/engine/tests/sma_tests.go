package tests

import (
	"tradovate-execution-engine/engine/indicators"
)

// RunSMATests executes all tests for the SMA indicator.
func RunSMATests() {
	testNewSMA()
	testSMABeforeFull()
	testSMAAtFull()
	testSMARolling()
	testSMAReset()
	testSMAGetHistorical()
}

func testNewSMA() {
	sma := indicators.NewSMA(5, indicators.OnBarClose)
	check("New SMA should have a current value of 0", sma.CurrentValue() == 0)
}

func testSMABeforeFull() {
	sma := indicators.NewSMA(5, indicators.OnBarClose)
	sma.Update(10)
	sma.Update(10)
	sma.Update(10)
	check("SMA value should be 0 before the period is full", sma.CurrentValue() == 0)
}

func testSMAAtFull() {
	sma := indicators.NewSMA(5, indicators.OnBarClose)
	sma.Update(1)
	sma.Update(2)
	sma.Update(3)
	sma.Update(4)
	val := sma.Update(5) // (1+2+3+4+5) / 5 = 3
	assertEqualsFloat("SMA should be correct when period is first met", 3.0, val, 0.001)
	assertEqualsFloat("SMA CurrentValue should be correct", 3.0, sma.CurrentValue(), 0.001)
}

func testSMARolling() {
	sma := indicators.NewSMA(3, indicators.OnBarClose)
	sma.Update(10)
	sma.Update(20)
	sma.Update(30) // SMA is (10+20+30)/3 = 20
	assertEqualsFloat("SMA should be 20 after 3 values", 20.0, sma.CurrentValue(), 0.001)

	// Add a new value, oldest (10) should be dropped
	sma.Update(40) // SMA is (20+30+40)/3 = 30
	assertEqualsFloat("SMA should roll correctly to 30", 30.0, sma.CurrentValue(), 0.001)

	// Add another value
	sma.Update(50) // SMA is (30+40+50)/3 = 40
	assertEqualsFloat("SMA should roll correctly to 40", 40.0, sma.CurrentValue(), 0.001)
}

func testSMAReset() {
	sma := indicators.NewSMA(3, indicators.OnBarClose)
	sma.Update(10)
	sma.Update(20)
	sma.Update(30)

	sma.Reset()
	check("SMA value should be 0 after reset", sma.CurrentValue() == 0)

	// After reset, it should fill up again
	sma.Update(100)
	sma.Update(100)
	val := sma.Update(100) // (100+100+100)/3 = 100
	assertEqualsFloat("SMA should be 100 after filling post-reset", 100.0, val, 0.001)
}

func testSMAGetHistorical() {
	sma := indicators.NewSMA(3, indicators.OnBarClose)
	sma.Update(10) // [10, 0, 0] count 1, last 0
	sma.Update(20) // [10, 20, 0] count 2, last 0
	sma.Update(30) // [10, 20, 30] count 3, last 20
	sma.Update(40) // [40, 20, 30] count 3, last 30
	sma.Update(50) // [40, 50, 30] count 3, last 40

	// Note: In SMA.go, lastValue is updated BEFORE being added to the ring buffer.
	// But Value.Get(0) returns s.lastValue.
	// Value.Get(1) returns values[(s.valueIdx - 1 - 1 + size) % size].
	
	assertEqualsFloat("Get(0) should be current value (40)", 40.0, sma.Value.Get(0), 0.001)
	assertEqualsFloat("Get(1) should be 1-ago value (30)", 30.0, sma.Value.Get(1), 0.001)
	assertEqualsFloat("Get(2) should be 2-ago value (20)", 20.0, sma.Value.Get(2), 0.001)
}
