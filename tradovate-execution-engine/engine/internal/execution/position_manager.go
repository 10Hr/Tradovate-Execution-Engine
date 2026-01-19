package execution

import (
	"encoding/json"
	"fmt"

	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/marketdata"
)

// NewPositionManager creates a new position manager
func NewPositionManager(client, mdClient marketdata.WebSocketSender, userID int) *PositionManager {
	return &PositionManager{
		client:      client,
		mdClient:    mdClient,
		pls:         make(map[string]*PositionPL),
		contractMap: make(map[int]string),
		productMap:  make(map[string]float64),
		userID:      userID,
	}
}

// SetLogger sets the logger for the position manager
func (pm *PositionManager) SetLogger(l *logger.Logger) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
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

// HandleUserSyncEvent processes the user sync response
func (pm *PositionManager) HandleUserSyncEvent(data json.RawMessage) {
	var syncData UserSyncData
	if err := json.Unmarshal(data, &syncData); err != nil {
		if pm.log != nil {
			pm.log.Errorf("Failed to unmarshal user sync data: %v", err)
		}
		return
	}

	// Initial response contains positions, contracts, and products
	if len(syncData.Users) > 0 {
		pm.processInitialSync(syncData)
	}
}

// processInitialSync handles the initial user sync response
func (pm *PositionManager) processInitialSync(syncData UserSyncData) {
	if pm.log != nil {
		pm.log.Infof("Received initial sync - Positions: %d, Contracts: %d, Products: %d",
			len(syncData.Positions), len(syncData.Contracts), len(syncData.Products))
	}

	pm.mu.Lock()
	// Populate lookup maps
	for _, contract := range syncData.Contracts {
		pm.contractMap[contract.ID] = contract.Name
	}

	for _, product := range syncData.Products {
		pm.productMap[product.Name] = product.ValuePerPoint
	}
	pm.mu.Unlock()

	// Process each position
	for _, pos := range syncData.Positions {
		// Skip if no position
		if pos.NetPos == 0 && pos.PrevPos == 0 {
			continue
		}

		pm.mu.RLock()
		// Get contract name
		contractName, ok := pm.contractMap[pos.ContractID]
		if !ok {
			if pm.log != nil {
				pm.log.Warnf("Contract ID %d not found in contracts", pos.ContractID)
			}
			pm.mu.RUnlock()
			continue
		}

		// Find matching product (products.name starts with contract name)
		var valuePerPoint float64
		for productName, vpp := range pm.productMap {
			if len(productName) >= len(contractName) && productName[:len(contractName)] == contractName {
				valuePerPoint = vpp
				break
			}
		}
		pm.mu.RUnlock()

		if valuePerPoint == 0 {
			if pm.log != nil {
				pm.log.Warnf("Value per point not found for contract %s", contractName)
			}
			continue
		}

		// Subscribe to market data for this position
		pm.subscribeToPosition(contractName, pos, valuePerPoint)
	}
}

// subscribeToPosition subscribes to market data for a position
func (pm *PositionManager) subscribeToPosition(contractName string, pos Position, valuePerPoint float64) {
	body := map[string]interface{}{
		"symbol": contractName,
	}

	if err := pm.mdClient.Send("md/subscribequote", body); err != nil {
		if pm.log != nil {
			pm.log.Errorf("Failed to subscribe to quotes for %s: %v", contractName, err)
		}
		return
	}

	if pm.log != nil {
		pm.log.Infof("Subscribed to market data for %s (NetPos: %d, NetPrice: %.2f, VPP: %.2f)",
			contractName, pos.NetPos, pos.NetPrice, valuePerPoint)
	}

	// Store position info for quote processing
	pm.mu.Lock()
	pm.pls[contractName] = &PositionPL{
		Name:          contractName,
		NetPos:        pos.NetPos,
		AvgPrice:      pos.NetPrice,
		ValuePerPoint: valuePerPoint,
	}
	pm.mu.Unlock()
}

// HandleQuoteUpdate processes quote updates and calculates P&L
func (pm *PositionManager) HandleQuoteUpdate(quote marketdata.Quote) {
	// Find position for this contract
	pm.mu.RLock()
	contractName, ok := pm.contractMap[quote.ContractID]
	if !ok {
		pm.mu.RUnlock()
		return
	}
	positionPL, exists := pm.pls[contractName]
	pm.mu.RUnlock()

	if !exists {
		// No position tracking for this symbol
		return
	}

	// Get the Trade entry from the quote
	trade, ok := quote.Entries["Trade"]
	if !ok {
		// No trade price available yet
		return
	}

	pm.calculateAndUpdatePL(contractName, trade.Price, positionPL.NetPos)
}

// calculateAndUpdatePL calculates and updates P&L for a position
func (pm *PositionManager) calculateAndUpdatePL(name string, currentPrice float64, netPos int) {
	pm.mu.Lock()
	positionPL, exists := pm.pls[name]
	if !exists {
		pm.mu.Unlock()
		return
	}

	if netPos == 0 {
		positionPL.PL = 0
	} else {
		// P&L = (Current - Entry) * Qty * VPP
		positionPL.PL = (currentPrice - positionPL.AvgPrice) * float64(netPos) * positionPL.ValuePerPoint
	}

	currentPL := positionPL.PL
	pm.mu.Unlock()

	if pm.log != nil {
		pm.log.Debugf("P&L Update - %s: $%.2f (Price: %.2f, Pos: %d)",
			name, currentPL, currentPrice, netPos)
	}

	// Call update callback
	if pm.OnPLUpdate != nil {
		pm.OnPLUpdate(name, currentPL, netPos)
	}

	// Calculate total P&L
	pm.runPL()
}

// runPL calculates and reports total P&L across all positions
func (pm *PositionManager) runPL() {
	totalPL := 0.0
	for _, positionPL := range pm.pls {
		totalPL += positionPL.PL
	}

	if pm.log != nil {
		pm.log.Infof("Total P&L: $%.2f", totalPL)
	}

	// Call total P&L callback
	if pm.OnTotalPLUpdate != nil {
		pm.OnTotalPLUpdate(totalPL)
	}
}

// GetPositionPL returns the P&L for a specific position
func (pm *PositionManager) GetPositionPL(name string) (float64, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if positionPL, exists := pm.pls[name]; exists {
		return positionPL.PL, true
	}
	return 0, false
}

// GetTotalPL returns the total P&L across all positions
func (pm *PositionManager) GetTotalPL() float64 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	totalPL := 0.0
	for _, positionPL := range pm.pls {
		totalPL += positionPL.PL
	}
	return totalPL
}

// GetAllPositions returns a copy of all position P&Ls
func (pm *PositionManager) GetAllPositions() map[string]PositionPL {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	positions := make(map[string]PositionPL)
	for name, positionPL := range pm.pls {
		positions[name] = *positionPL
	}
	return positions
}

// HandlePositionUpdate processes real-time position updates
func (pm *PositionManager) HandlePositionUpdate(pos APIPosition) {
	pm.mu.RLock()
	contractName, ok := pm.contractMap[pos.ContractID]
	pm.mu.RUnlock()

	if !ok {
		if pm.log != nil {
			pm.log.Warnf("HandlePositionUpdate: Contract ID %d not found", pos.ContractID)
		}
		return
	}

	pm.mu.Lock()
	// Update existing position
	if positionPL, exists := pm.pls[contractName]; exists {
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
		pm.mu.Unlock()
		return
	}
	pm.mu.Unlock()
}
