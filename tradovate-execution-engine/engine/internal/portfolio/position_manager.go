package portfolio

import (
	"fmt"

	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/marketdata"
	"tradovate-execution-engine/engine/internal/tradovate"
)

// NewPositionManager creates a new position manager
func NewPositionManager(client, mdClient marketdata.WebSocketSender, userID int) *PositionManager {
	return &PositionManager{
		client:      client,
		mdClient:    mdClient,
		Pls:         make(map[string]*PositionPL),
		contractMap: make(map[int]string),
		productMap:  make(map[string]float64),
		userID:      userID,
	}
}

// SetLogger sets the logger for the position manager
func (pm *PositionManager) SetLogger(l *logger.Logger) {
	pm.Mu.Lock()
	defer pm.Mu.Unlock()
	pm.log = l
}

// Start begins tracking positions and P&L
func (pm *PositionManager) Start() error {
	body := map[string]interface{}{
		"users": []int{pm.userID},
	}

	if err := pm.client.Send("user/syncrequest", body); err != nil {
		return fmt.Errorf("failed to send user sync request: %w", err)
	}

	if pm.log != nil {
		pm.log.Info("Position manager started - awaiting user sync data")
	}

	return nil
}

// GetPositionPL returns the P&L for a specific position
func (pm *PositionManager) GetPositionPL(name string) (float64, bool) {
	pm.Mu.RLock()
	defer pm.Mu.RUnlock()

	if positionPL, exists := pm.Pls[name]; exists {
		return positionPL.PL, true
	}
	return 0, false
}

// GetTotalPL returns the total P&L across all positions
func (pm *PositionManager) GetTotalPL() float64 {
	pm.Mu.RLock()
	defer pm.Mu.RUnlock()

	totalPL := 0.0
	for _, positionPL := range pm.Pls {
		totalPL += positionPL.PL
	}
	return totalPL
}

// GetAllPositions returns a copy of all position P&Ls
func (pm *PositionManager) GetAllPositions() map[string]PositionPL {
	pm.Mu.RLock()
	defer pm.Mu.RUnlock()

	positions := make(map[string]PositionPL)
	for name, positionPL := range pm.Pls {
		positions[name] = *positionPL
	}
	return positions
}

// HandlePositionUpdate processes real-time position updates
func (pm *PositionManager) HandlePositionUpdate(pos tradovate.APIPosition) {
	pm.log.Infof("ContractID: %d, ID: %d, NetPos: %d, NetPrice: %.2f, BoughtValue: %.2f",
		pos.ContractID, pos.ID, pos.NetPos, pos.NetPrice, pos.BoughtValue)
	pm.Mu.RLock()
	contractName, ok := pm.contractMap[pos.ID]
	pm.Mu.RUnlock()

	if !ok {
		if pm.log != nil {
			pm.log.Warnf("HandlePositionUpdate: Contract ID %d not found", pos.ContractID)
		}
		return
	}

	pm.Mu.Lock()
	// Update existing position
	if positionPL, exists := pm.Pls[contractName]; exists {
		positionPL.NetPos = pos.NetPos
		positionPL.AvgPrice = pos.NetPrice

		// Realized PnL Calculation from API
		// Cash Flow = SoldValue - BoughtValue
		// Cost of Open Position = NetPos * NetPrice
		// Realized PnL = Cash Flow + Cost of Open Position
		realizedPnLPoints := (pos.SoldValue - pos.BoughtValue) + (float64(pos.NetPos) * pos.NetPrice)
		positionPL.RealizedPL = realizedPnLPoints * positionPL.ValuePerPoint

		if pm.log != nil {
			pm.log.Infof("Updated position %s: NetPos=%d, AvgPrice=%.2f, RealizedPL=%.2f",
				contractName, pos.NetPos, pos.NetPrice, positionPL.RealizedPL)
		}
		pm.Mu.Unlock()
		return
	}
	pm.Mu.Unlock()
}
