package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"tradovate-execution-engine/engine/config"
)

var (
	totalTests  int
	failedTests int
	testLog     *os.File
)

// RunAllTests is the master test function that calls all other test suites.
func RunAllTests() {
	path := filepath.Join(config.GetProjectRoot(), "engine", "tests", "tests.txt")
	var err error
	testLog, err = os.Create(path)
	if err != nil {
		fmt.Printf("Error creating test log at %s: %v\n", path, err)
		return
	}
	defer testLog.Close()

	logPrint("Running All Tests...")
	logPrint("=======================================")

	runTest("SMA Indicator Tests", RunSMATests)
	logPrint("\n")
	runTest("MA Crossover Strategy Tests", RunMACrossoverTests)
	logPrint("\n")
	runTest("Risk Management Tests", RunRiskTests)

	logPrint("=======================================")
	logPrintf("Test Run Complete. Total: %d, Passed: %d, Failed: %d\n", totalTests, totalTests-failedTests, failedTests)

	if failedTests > 0 {
		os.Exit(1) // Exit with a non-zero code to indicate failure
	}
}

// logPrint writes to both console and the test file
func logPrint(args ...interface{}) {
	msg := fmt.Sprintln(args...)
	fmt.Print(msg)
	if testLog != nil {
		testLog.WriteString(msg)
	}
}

// logPrintf writes formatted string to both console and the test file
func logPrintf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Print(msg)
	if testLog != nil {
		testLog.WriteString(msg)
	}
}

// runTest is a helper to execute a test suite function and print the results.
func runTest(name string, testFunc func()) {
	logPrintf("--- Running: %s ---\n", name)
	testFunc()
	logPrintf("--- Finished: %s ---\n", name)
}

// check is a helper function to assert a condition.
func check(name string, condition bool) {
	totalTests++
	if condition {
		logPrintf("  ✅ PASS: %s\n", name)
	} else {
		failedTests++
		logPrintf("  ❌ FAIL: %s\n", name)
	}
}

// assertEqualsFloat is a helper for checking float64 equality with a tolerance.
func assertEqualsFloat(name string, expected, actual, tolerance float64) {
	totalTests++
	if abs(expected-actual) < tolerance {
		logPrintf("  ✅ PASS: %s (Expected: %.2f, Got: %.2f)\n", name, expected, actual)
	} else {
		failedTests++
		logPrintf("  ❌ FAIL: %s (Expected: %.2f, Got: %.2f)\n", name, expected, actual)
	}
}

// abs returns the absolute value of a float64.
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
