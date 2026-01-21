package execution

import (
	"fmt"
	"sync"
)

// StrategyParam represents a single configuration parameter for a strategy
type StrategyParam struct {
	Name        string
	Type        string // "int", "float", "string"
	Value       string
	Description string
}

// Strategy interface defines the required metohds for any trading strategy
type Strategy interface {
	Name() string
	Description() string
	GetParams() []StrategyParam
	SetParam(name, value string) error
	Init(om *OrderManager) error
	OnTick(price float64) error
	Reset()
}

// StrategyRegistry maintains a list of avaialble strategies
type StrategyRegistry struct {
	mu         sync.RWMutex
	strategies map[string]func() Strategy
}

var globalRegistry = &StrategyRegistry{
	strategies: make(map[string]func() Strategy),
}

// Register adds a strategy to the global registry
func Register(name string, factory func() Strategy) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.strategies[name] = factory
}

// GetAvailableStrategies returns a list of registered strategy names
func GetAvailableStrategies() []string {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	names := make([]string, 0, len(globalRegistry.strategies))
	for name := range globalRegistry.strategies {
		names = append(names, name)
	}
	return names
}

// CreateStrategy instantiates a strategy by name
func CreateStrategy(name string) (Strategy, error) {
	globalRegistry.mu.RLock()
	factory, exists := globalRegistry.strategies[name]
	globalRegistry.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("strategy not found: %s", name)
	}
	return factory(), nil
}
