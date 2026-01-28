package execution

import (
	"fmt"
	"tradovate-execution-engine/engine/internal/logger"
)

// Register adds a strategy to the global registry
func Register(name string, factory func(*logger.Logger) Strategy) {
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
func CreateStrategy(name string, logger *logger.Logger) (Strategy, error) {
	globalRegistry.mu.RLock()
	factory, exists := globalRegistry.strategies[name]
	globalRegistry.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("strategy not found: %s", name)
	}
	return factory(logger), nil
}
