package tests

import (
	"fmt"
	"tradovate-execution-engine/engine/indicators"
	"tradovate-execution-engine/engine/strategies"
)

// RunMACrossoverTests executes all tests for the MACrossover strategy logic.
func RunMACrossoverTests() {
	testCrossAbove()
	testCrossBelow()
}

func testCrossAbove() {
	// Fast(3), Slow(5)
	strategy := strategies.NewMACrossover("MESH6", 3, 5, indicators.OnBarClose)
	strategy.Init(nil)

	// 1. Initial prices (filling up)
	prices := []float64{10, 10, 10, 10, 10}
	for i, p := range prices {
		strategy.OnBar(fmt.Sprintf("T%d", i), p)
	}
	// Fast SMA: 10, Slow SMA: 10. No cross yet.
	check("No cross above when SMAs are equal", !strategy.CrossAbove(1))

	// 2. Prices move to separate SMAs
	strategy.OnBar("T5", 12) // Fast: (10+10+12)/3=10.67, Slow: (10+10+10+10+12)/5=10.4.
	// Current: Fast(10.67) > Slow(10.4). Previous: Fast(10) == Slow(10).
	// This IS a cross above because Prev <= Prev and Now > Now.
	check("Cross above detected when fast moves above slow", strategy.CrossAbove(1))
}

func testCrossBelow() {
	// Fast(3), Slow(5)
	strategy := strategies.NewMACrossover("MESH6", 3, 5, indicators.OnBarClose)
	strategy.Init(nil)

	// 1. Initial prices
	prices := []float64{20, 20, 20, 20, 20}
	for i, p := range prices {
		strategy.OnBar(fmt.Sprintf("T%d", i), p)
	}

	// 2. Fast moves below
	strategy.OnBar("T5", 15) // Fast: (20+20+15)/3=18.33, Slow: (20+20+20+20+15)/5=19.
	// Current: Fast(18.33) < Slow(19). Previous: Fast(20) == Slow(20).
	check("Cross below detected when fast moves below slow", strategy.CrossBelow(1))
}
