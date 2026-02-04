package UI

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"tradovate-execution-engine/engine/config"
	"tradovate-execution-engine/engine/internal/auth"
	"tradovate-execution-engine/engine/internal/execution"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/marketdata"
	"tradovate-execution-engine/engine/internal/models"
	"tradovate-execution-engine/engine/internal/portfolio"
	"tradovate-execution-engine/engine/internal/tradovate"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Styles
var (
	tabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Bold(true)

	activeTabStyle = tabStyle.Copy().
			Foreground(lipgloss.Color("36")).
			Background(lipgloss.Color("235"))

	inactiveTabStyle = tabStyle.Copy().
				Foreground(lipgloss.Color("240"))

	contentStyle = lipgloss.NewStyle().
			Padding(1, 2)

	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("230")).
			Padding(0, 1)

	commandBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("234")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("46"))

	logPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	menuItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)

	disabledStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
)

var (
	sessionStart time.Time
)

const (
	TabMain Tab = iota
	TabStrategy
	TabOrderManagement
	TabPositions
	TabCommands
	configFile = "config.json"
)

const (
	ModeVisual TradingMode = iota
	ModeLive
)

const (
	modeNormal mode = iota
	modeCommand
	modeEditor
)

const (
	StrategyDisabled StrategyStatus = iota
	StrategyStarting
	StrategyRunning
	StrategyStopping
	StrategyStopped
	StrategyError
)

func InitialModel() model {
	// Create Loggers

	logLevel := logger.LevelInfo
	mainLog := logger.NewLogger(500, logLevel)
	orderLog := logger.NewLogger(500, logLevel)
	strategyLog := logger.NewLogger(500, logLevel)

	// Initial log messages
	mainLog.Println("System initialized")
	mainLog.Printf("Starting trading engine v1.0.0")

	// Initialize Editor
	ta := textarea.New()
	ta.Placeholder = "Config content..."
	ta.Focus()

	availableStrats := execution.GetAvailableStrategies()
	mainLog.Infof("Discovered %d registered strategies: %v", len(availableStrats), availableStrats)

	return model{
		activeTab:      TabMain,
		mode:           modeNormal,
		tradingMode:    ModeVisual,
		connected:      false,
		totalPnL:       0,
		mainLogger:     mainLog,
		orderLogger:    orderLog,
		strategyLogger: strategyLog,

		configPath:           config.GetConfigPath(),
		strategyName:         "No strategy selected",
		logScrollOffset:      1000000,
		orderLogScrollOffset: 1000000,
		stratLogScrollOffset: 1000000,
		commandHistory:       []string{},
		historyIndex:         0,
		configEditor:         ta,
		availableStrategies:  availableStrats,
		strategyParams:       make(map[string]string),

		// Empty data - will be populated from OrderManager
		positions:  []PositionRow{},
		orders:     []OrderRow{},
		pnlHistory: []PnLDataPoint{},
		commands: []Command{
			{Name: "buy", Description: "Place a buy order", Usage: ":buy <symbol> <quantity>", Category: "Trading"},
			{Name: "sell", Description: "Place a sell order", Usage: ":sell <symbol> <quantity>", Category: "Trading"},
			{Name: "flatten", Description: "Flatten all positions", Usage: ":flatten", Category: "Trading"},
			{Name: "mode", Description: "Switch trading mode (live/visual)", Usage: ":mode <live|visual> or mode <l|v>", Category: "System"},
			{Name: "config", Description: "Edit configuration", Usage: ":config", Category: "System"},
			{Name: "strategy", Description: "Select strategy", Usage: ":strategy <name>", Category: "System"},
			{Name: "export", Description: "Export logs", Usage: ":export <log|orders|strat>", Category: "System"},
			{Name: "help", Description: "Show commands page", Usage: ":help", Category: "Navigation"},
			{Name: "quit", Description: "Exit the application", Usage: ":quit or :q", Category: "System"},
		},
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (r *StrategyRuntime) SetStatus(s StrategyStatus) {
	r.status.Store(int32(s))
}

func (r *StrategyRuntime) Status() StrategyStatus {
	return StrategyStatus(r.status.Load())
}

func (m model) Init() tea.Cmd {
	return tickCmd()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		// Update data from OrderManager
		if m.om != nil {
			// Update Orders
			execOrders := m.om.GetAllOrders()
			uiOrders := make([]OrderRow, len(execOrders))
			for i, o := range execOrders {
				uiOrders[i] = OrderRow{
					ID:       o.ID,
					Symbol:   o.Symbol,
					Side:     string(o.Side),
					Quantity: o.Quantity,
					Price:    o.Price,
					Status:   string(o.Status),
					Time:     o.SubmittedAt,
				}
			}
			m.orders = uiOrders

			// Use PortfolioTracker as the source of truth if available
			if m.pt != nil {
				summary := m.pt.GetPLSummary()
				var uiPositions []PositionRow
				var unrealizedTotal float64

				for _, entry := range summary {
					if entry.NetPos != 0 {
						uiPositions = append(uiPositions, PositionRow{
							Symbol:   entry.Name,
							Quantity: entry.NetPos,
							AvgPrice: entry.BuyPrice,
							PnL:      entry.PL,
						})
					}
					unrealizedTotal += entry.PL
				}
				m.positions = uiPositions
				m.unrealizedPnL = m.pt.GetTotalPL()
				m.dailyrealizedPnL = m.pt.GetRealizedPnL()
				m.realizedPnL = m.pt.GetSessionRealizedPnL()
				m.totalPnL = m.unrealizedPnL + m.dailyrealizedPnL

				// Check daily loss limit
				if m.om != nil && m.om.GetRiskManager().IsDailyLossExceeded(m.totalPnL) {
					// 1. Always flatten if we have open positions
					if len(m.positions) > 0 {
						m.mainLogger.Error("Daily loss limit exceeded! Flattening all positions.")
						m.om.FlattenPositions()
						m.statusMsg = errorStyle.Render("DAILY LOSS LIMIT REACHED - POSITIONS FLATTENED")
					}

					// 2. Stop the strategy if it's running
					if m.currentStrategy != nil && m.currentStrategy.Runtime.Status() == StrategyRunning {
						m.mainLogger.Error("Daily loss limit exceeded! Stopping strategy.")
						m.stopCurrentStrategy()
						m.statusMsg = errorStyle.Render("DAILY LOSS LIMIT REACHED - STRATEGY STOPPED")
					}
				}
			}

			// Update PnL History (simple version: append every tick if changed or every X seconds)
			now := time.Now()
			if len(m.pnlHistory) == 0 || now.Sub(m.pnlHistory[len(m.pnlHistory)-1].Time) > 10*time.Second {
				m.pnlHistory = append(m.pnlHistory, PnLDataPoint{
					Time: now,
					PnL:  m.totalPnL,
				})
				// Keep history limited
				if len(m.pnlHistory) > 100 {
					m.pnlHistory = m.pnlHistory[1:]
				}
			}
		}

		return m, tickCmd()

	case connMsg:
		if msg.err != nil {
			m.mainLogger.Errorf("Connection error: %v", msg.err)
			m.statusMsg = errorStyle.Render("Connection failed")
			m.connected = false
		}
		return m, nil

	case connMsgSuccess:
		m.config = msg.config
		m.tm = msg.tokenManager
		m.om = msg.orderManager
		m.marketDataClient = msg.mdClient
		m.marketDataSubscriptionManager = msg.mdSubscriber
		m.tradingClient = msg.tradingClient
		m.tradingClientSubscriptionManager = msg.tradingSubscriber
		m.pt = msg.portfolioTracker
		m.connected = true

		m.mainLogger.Info(">>> CONNECTION SUCCESSFUL <<<")
		m.statusMsg = successStyle.Render("Connected to Tradovate")
		return m, nil

	case editorFinishedMsg:
		if msg.err != nil {
			m.mainLogger.Errorf("Editor error: %v", msg.err)
			m.statusMsg = errorStyle.Render("Failed to open editor")
		} else {
			m.mainLogger.Info(">>> Config editor closed, proceeding... <<<")
			if msg.nextAction == "connect" {
				return m, m.connectCmd()
			}
		}
		return m, nil
	}

	return m, nil
}

func (m model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keybindings
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	switch m.mode {
	case modeNormal:
		return m.handleNormalMode(msg)
	case modeCommand:
		return m.handleCommandMode(msg)
	case modeEditor:
		return m.handleEditorMode(msg)
	}

	return m, nil
}

func (m model) handleNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {

	// If on commands tab and search is active, handle search input
	if m.activeTab == TabCommands && m.searchActive {
		return m.handleSearchInput(msg)
	}

	switch msg.String() {
	case "q":
		if m.activeTab != TabMain {
			return m, tea.Quit
		}

	case "a":
		if m.activeTab > 0 {
			m.activeTab--
			m.scrollOffset = 0
		}

	case "d":
		if m.activeTab < TabCommands {
			m.activeTab++
			m.scrollOffset = 0
		}

	case "1":
		m.activeTab = TabMain
		m.scrollOffset = 0

	case "2":
		m.activeTab = TabStrategy
		m.scrollOffset = 0

	case "3":
		m.activeTab = TabOrderManagement
		m.scrollOffset = 0

	case "4":
		m.activeTab = TabPositions
		m.scrollOffset = 0

	case "5":
		m.activeTab = TabCommands
		m.scrollOffset = 0

	case ":", "/":
		m.mode = modeCommand
		m.commandInput = ":"

	// Shift Commands (Main Menu Actions)
	case "!": // Shift+1
		if m.connected {
			m.mainLogger.Info(">>> DISCONNECTING... <<<")
			m.connected = false

			m.stopCurrentStrategy()

			if m.tm != nil {
				m.tm.StopTokenRefreshMonitor()
			}

			if m.pt != nil {
				_ = m.pt.Stop()
				m.pt = nil
			}

			if m.marketDataSubscriptionManager != nil {
				_ = m.marketDataSubscriptionManager.UnsubscribeAll()
			}

			if m.marketDataClient != nil {
				_ = m.marketDataClient.Disconnect()
				m.marketDataClient = nil
			}

			if m.tradingClient != nil {
				_ = m.tradingClient.Disconnect()
				m.tradingClient = nil
			}

			m.statusMsg = errorStyle.Render("Disconnected from Tradovate")
			m.mainLogger.Info(">>> SUCCESSFULLY DISCONNECTED <<<")
			return m, nil
		} else {
			m.mainLogger.Info(">>> STARTING CONNECTION SEQUENCE... <<<")

			// Check if config exists
			if _, err := os.Stat(m.configPath); os.IsNotExist(err) {

				config, _ := config.LoadOrCreateConfig(m.mainLogger)

				// Open editor instead of trying to open external notepad
				content, _ := os.ReadFile(m.configPath)
				m.configEditor.SetValue(string(content))
				m.configEditor.SetWidth(m.width - 4)
				m.configEditor.SetHeight(m.height - 10)
				m.mode = modeEditor

				m.config = config

				return m, nil
			}

			return m, m.connectCmd()
		}
	case "@": // Shift+2
		if !m.connected {
			return m, nil
		}
		m.mode = modeCommand
		m.commandInput = ":strategy "
		m.statusMsg = "Enter strategy name..."
	case "#": // Shift+3
		var content string
		var logName string

		switch m.activeTab {
		case TabStrategy:
			m.mainLogger.Info(">>> EXPORTING STRATEGY LOG TO FILE... <<<")
			content = m.strategyLogger.ExportToString()
			logName = "strat_log_"
		case TabOrderManagement:
			m.mainLogger.Info(">>> EXPORTING ORDER LOG TO FILE... <<<")
			content = m.orderLogger.ExportToString()
			logName = "order_log_"
		default:
			m.mainLogger.Info(">>> EXPORTING MAIN LOG TO FILE... <<<")
			content = m.mainLogger.ExportToString()
			logName = "main_log_"
		}

		logsDir := filepath.Join(config.GetProjectRoot(), "external", "logs")
		_ = os.MkdirAll(logsDir, 0755)
		filename := filepath.Join(logsDir, logName+time.Now().Format("01-02-2006_3-04-05_PM")+".txt")
		err := os.WriteFile(filename, []byte(content), 0644)
		if err != nil {
			m.statusMsg = errorStyle.Render("Export failed: " + err.Error())
			m.mainLogger.Errorf("Export failed: %v", err)
		} else {
			m.statusMsg = successStyle.Render("Log exported to " + filename)
			m.mainLogger.Printf("Log successfully exported to %s", filename)
		}

	case "$": // Shift+4
		content, err := os.ReadFile(m.configPath)
		if err != nil {
			// Try to create it if it doesn't exist
			if os.IsNotExist(err) {
				_ = config.CreateDefaultConfig(m.configPath)
				content, _ = os.ReadFile(m.configPath)
			} else {
				m.statusMsg = errorStyle.Render("Failed to read config: " + err.Error())
				return m, nil
			}
		}
		m.configEditor.SetValue(string(content))
		m.configEditor.SetWidth(m.width - 4)
		m.configEditor.SetHeight(m.height - 10)
		m.mode = modeEditor
		m.statusMsg = ""
		m.mainLogger.Printf(">>> CONFIG EDITOR OPENED: %s <<<", m.configPath)
	case "%": // Shift+5
		m.mainLogger.Clear()
		m.logScrollOffset = 0
		m.mainLogger.Info(">>> LOG CLEARED <<<")

	case "ctrl+f", "f":
		// Activate search on commands page
		if m.activeTab == TabCommands {
			m.searchActive = true
			m.searchInput = ""
		}

	case "w", "up":
		// Scroll up based on context
		switch m.activeTab {
		case TabMain:
			// Calculate max scroll for Main Log to clamp "infinity"
			availableLines := m.height - 11
			if availableLines < 1 {
				availableLines = 1
			}

			entriesLen := m.mainLogger.Count()
			maxScroll := entriesLen - availableLines
			if maxScroll < 0 {
				maxScroll = 0
			}

			// Clamp if we are past the bottom (e.g. from auto-scroll)
			if m.logScrollOffset > maxScroll {
				m.logScrollOffset = maxScroll
			}

			if m.logScrollOffset > 0 {
				m.logScrollOffset--
			}
		case TabOrderManagement:
			// Calculate max scroll for Order Log
			availableLines := m.height - 11
			if availableLines < 1 {
				availableLines = 1
			}

			entriesLen := m.orderLogger.Count()
			maxScroll := entriesLen - availableLines
			if maxScroll < 0 {
				maxScroll = 0
			}

			// Clamp
			if m.orderLogScrollOffset > maxScroll {
				m.orderLogScrollOffset = maxScroll
			}

			if m.orderLogScrollOffset > 0 {
				m.orderLogScrollOffset--
			}
		case TabStrategy:
			// Calculate max scroll for Main Log to clamp "infinity"
			availableLines := m.height - 11
			if availableLines < 1 {
				availableLines = 1
			}

			entriesLen := m.strategyLogger.Count()
			maxScroll := entriesLen - availableLines
			if maxScroll < 0 {
				maxScroll = 0
			}

			// Clamp if we are past the bottom (e.g. from auto-scroll)
			if m.stratLogScrollOffset > maxScroll {
				m.stratLogScrollOffset = maxScroll
			}

			if m.stratLogScrollOffset > 0 {
				m.stratLogScrollOffset--
			}
		default:
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		}

	case "s", "down":
		// Scroll down based on context
		switch m.activeTab {
		case TabMain:
			// Calculate max scroll for Main Log
			availableLines := m.height - 11
			if availableLines < 1 {
				availableLines = 1
			}

			entriesLen := m.mainLogger.Count()
			maxScroll := entriesLen - availableLines
			if maxScroll < 0 {
				maxScroll = 0
			}

			if m.logScrollOffset < maxScroll {
				m.logScrollOffset++
			}

		case TabOrderManagement:
			// Calculate max scroll for Order Log
			availableLines := m.height - 11
			if availableLines < 1 {
				availableLines = 1
			}

			entriesLen := m.orderLogger.Count()
			maxScroll := entriesLen - availableLines
			if maxScroll < 0 {
				maxScroll = 0
			}

			if m.orderLogScrollOffset < maxScroll {
				m.orderLogScrollOffset++
			}

		case TabStrategy:
			// Calculate max scroll for Order Log
			availableLines := m.height - 11
			if availableLines < 1 {
				availableLines = 1
			}

			entriesLen := m.strategyLogger.Count()
			maxScroll := entriesLen - availableLines
			if maxScroll < 0 {
				maxScroll = 0
			}

			if m.stratLogScrollOffset < maxScroll {
				m.stratLogScrollOffset++
			}

		default:
			contentHeight := m.height - 5
			fullContent := m.renderCommandsContent()
			totalLines := len(strings.Split(fullContent, "\n"))
			maxScroll := totalLines - contentHeight
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.scrollOffset < maxScroll {
				m.scrollOffset++
			}
		}

	case "W":
		// Go to top (Shift+W)
		switch m.activeTab {
		case TabMain:
			m.logScrollOffset = 0
		case TabOrderManagement:
			m.orderLogScrollOffset = 0
		case TabStrategy:
			m.stratLogScrollOffset = 0
		default:
			m.scrollOffset = 0
		}

	case "S":
		// Go to bottom (Shift+S)
		switch m.activeTab {
		case TabMain:
			availableLines := m.height - 11
			if availableLines < 1 {
				availableLines = 1
			}

			entriesLen := m.mainLogger.Count()
			maxScroll := entriesLen - availableLines
			if maxScroll < 0 {
				maxScroll = 0
			}

			m.logScrollOffset = maxScroll

		case TabOrderManagement:
			availableLines := m.height - 11
			if availableLines < 1 {
				availableLines = 1
			}

			entriesLen := m.orderLogger.Count()
			maxScroll := entriesLen - availableLines
			if maxScroll < 0 {
				maxScroll = 0
			}

			m.orderLogScrollOffset = maxScroll
		case TabStrategy:
			availableLines := m.height - 11
			if availableLines < 1 {
				availableLines = 1
			}

			entriesLen := m.strategyLogger.Count()
			maxScroll := entriesLen - availableLines
			if maxScroll < 0 {
				maxScroll = 0
			}

			m.stratLogScrollOffset = maxScroll
		default:
			contentHeight := m.height - 5
			fullContent := m.renderCommandsContent()
			totalLines := len(strings.Split(fullContent, "\n"))
			maxScroll := totalLines - contentHeight
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.scrollOffset = maxScroll
		}
	}

	return m, nil
}

func (m model) handleSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.searchActive = false
		m.searchInput = ""

	case tea.KeyCtrlA:
		// Select all - not needed for our simple search, but we'll ignore it
		return m, nil

	case tea.KeyCtrlC:
		// Copy - in a terminal this would be handled by the terminal itself
		return m, nil

	case tea.KeyCtrlV:
		// Paste - in a terminal this would be handled by the terminal itself
		return m, nil

	case tea.KeyBackspace:
		if len(m.searchInput) > 0 {
			m.searchInput = m.searchInput[:len(m.searchInput)-1]
		}

	default:
		// Allow all printable characters including space
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			m.searchInput += msg.String()
		}
	}

	return m, nil
}

func (m model) handleCommandMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = modeNormal
		m.commandInput = ""

	case tea.KeyEnter:
		// Save to history if not empty and not identical to last entry
		rawCmd := strings.TrimPrefix(m.commandInput, ":")
		if rawCmd != "" {
			if len(m.commandHistory) == 0 || m.commandHistory[len(m.commandHistory)-1] != rawCmd {
				m.commandHistory = append(m.commandHistory, rawCmd)
			}
		}
		m.historyIndex = len(m.commandHistory)

		var cmd tea.Cmd
		m, cmd = m.executeCommand()
		m.mode = modeNormal
		m.commandInput = ""
		return m, cmd

	case tea.KeyUp:
		if m.historyIndex > 0 {
			m.historyIndex--
			m.commandInput = ":" + m.commandHistory[m.historyIndex]
		}

	case tea.KeyDown:
		if m.historyIndex < len(m.commandHistory)-1 {
			m.historyIndex++
			m.commandInput = ":" + m.commandHistory[m.historyIndex]
		} else if m.historyIndex == len(m.commandHistory)-1 {
			m.historyIndex = len(m.commandHistory)
			m.commandInput = ":"
		}

	case tea.KeyBackspace:
		if len(m.commandInput) > 1 {
			m.commandInput = m.commandInput[:len(m.commandInput)-1]
		}

	default:
		if msg.Type == tea.KeyRunes {
			m.commandInput += msg.String()
		}
	}

	return m, nil
}

func (m *model) handleEditorMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.isLogView {
		switch msg.Type {
		case tea.KeyCtrlE:
			// Export to file
			content := m.configEditor.Value()
			filename := "exported_log_" + time.Now().Format("20060102_150405") + ".txt"
			err := os.WriteFile(filename, []byte(content), 0644)
			if err != nil {
				m.statusMsg = errorStyle.Render("Export failed: " + err.Error())
			} else {
				m.statusMsg = successStyle.Render("Log exported to " + filename)
			}
			return *m, nil

		case tea.KeyCtrlC:
			err := clipboard.WriteAll(m.configEditor.Value())
			if err == nil {
				m.statusMsg = "Copied all log text to clipboard"
			}
			return *m, nil

		case tea.KeyCtrlV:
			text, err := clipboard.ReadAll()
			if err == nil {
				m.configEditor.InsertString(text)
				m.statusMsg = "Pasted into log view"
			}
			return *m, nil

		case tea.KeyCtrlA:
			m.configEditor.SetCursor(len(m.configEditor.Value()))
			m.statusMsg = "Cursor moved to end of log"
			return *m, nil

		case tea.KeyEsc:
			m.mode = modeNormal
			m.isLogView = false
			return *m, nil
		}
		// In log view, we don't allow typing other than the specific shortcuts
		return *m, nil
	}

	// Config Editor Logic
	switch msg.Type {
	case tea.KeyCtrlS:
		// Save content
		err := os.WriteFile(m.configPath, []byte(m.configEditor.Value()), 0644)
		if err != nil {
			m.statusMsg = errorStyle.Render("Failed to save config: " + err.Error())
			m.mainLogger.Errorf("Save failed: %v", err)
		} else {
			if m.connected {
				m.statusMsg = successStyle.Render("Config saved. Reconnect (!) to apply changes.")
			} else {
				m.statusMsg = successStyle.Render("Config saved successfully")
			}
			m.mainLogger.Info("Config saved via integrated editor")
		}
		return *m, nil

	case tea.KeyCtrlC:
		err := clipboard.WriteAll(m.configEditor.Value())
		if err == nil {
			m.statusMsg = "Copied all text to clipboard"
		}
		return *m, nil

	case tea.KeyCtrlV:
		text, err := clipboard.ReadAll()
		if err == nil {
			m.configEditor.InsertString(text)
			m.statusMsg = "Pasted from clipboard"
		}
		return *m, nil

	case tea.KeyCtrlA:
		m.configEditor.SetCursor(len(m.configEditor.Value()))
		m.statusMsg = "Cursor moved to end of text"
		return *m, nil

	case tea.KeyEsc:
		m.mode = modeNormal
		return *m, nil

	case tea.KeyRunes:
		if msg.String() == ":" {
			m.mode = modeCommand
			m.commandInput = ":"
			return *m, nil
		}
	}

	var cmd tea.Cmd
	m.configEditor, cmd = m.configEditor.Update(msg)
	return *m, cmd
}

func (m model) executeCommand() (model, tea.Cmd) {
	// Force scroll to bottom when a command is executed
	m.logScrollOffset = 1000000
	m.orderLogScrollOffset = 1000000
	m.stratLogScrollOffset = 1000000

	cmd := strings.TrimPrefix(m.commandInput, ":")
	parts := strings.Fields(cmd)

	if len(parts) == 0 {
		return m, nil
	}

	switch parts[0] {
	case "q", "quit", "Q":
		return m, tea.Quit

	case "buy", "sell":
		if !m.connected {
			m.statusMsg = errorStyle.Render("Must be connected to API to trade")
			return m, nil
		}
		if m.om != nil && m.om.GetRiskManager().IsDailyLossExceeded(m.totalPnL) {
			m.statusMsg = errorStyle.Render("Trading disabled: daily loss limit exceeded")
			return m, nil
		}
		if m.tradingMode != ModeLive {
			m.statusMsg = errorStyle.Render("Cannot place orders in Visual mode. Switch to Live mode with :mode live")
			m.mainLogger.Errorf("Order rejected: Not in Live mode")
			return m, nil
		}
		if len(parts) < 3 {
			m.statusMsg = errorStyle.Render("Usage: :" + parts[0] + " <symbol> <quantity>")
			return m, nil
		}

		symbol := parts[1]
		qtyStr := parts[2]

		if !strings.HasPrefix(qtyStr, "") {
			m.statusMsg = errorStyle.Render("Usage: :" + parts[0] + " <symbol> <quantity>")
			return m, nil
		}

		var qty int
		if _, err := fmt.Sscanf(qtyStr, "%d", &qty); err != nil {
			m.statusMsg = errorStyle.Render("Invalid quantity format. Use a number")
			return m, nil
		}

		if m.om == nil {
			m.statusMsg = errorStyle.Render("Order Manager not initialized")
			m.mainLogger.Error("Order Manager not initialized")
			return m, nil
		}

		side := models.SideBuy
		if parts[0] == "sell" {
			side = models.SideSell
		}

		m.mainLogger.Printf("Submitting %s order for %d %s...", strings.ToUpper(parts[0]), qty, symbol)

		order, err := m.om.SubmitMarketOrder(symbol, side, qty)
		if err != nil {
			m.statusMsg = errorStyle.Render("Order failed: " + err.Error())
			m.mainLogger.Errorf("Order failed: %v", err)
			return m, nil
		}

		m.statusMsg = successStyle.Render(fmt.Sprintf("%s order placed for %s (ID: %s)", strings.ToUpper(parts[0]), symbol, order.ID))
		m.mainLogger.Printf("%s order placed for %s (ID: %s)", strings.ToUpper(parts[0]), symbol, order.ID)
		m.orderLogger.Printf("%s %s - Price: Market, Qty: %d, ID: %s", strings.ToUpper(parts[0]), symbol, qty, order.ID)

	case "flatten":
		if !m.connected {
			m.statusMsg = errorStyle.Render("Must be connected to API to flatten positions")
			return m, nil
		}
		if m.tradingMode != ModeLive {
			m.statusMsg = errorStyle.Render("Cannot flatten in Visual mode")
			m.mainLogger.Errorf("Flatten rejected: Not in Live mode")
			return m, nil
		}

		if m.om != nil {
			if len(m.positions) == 0 {
				m.mainLogger.Error("Flatten failed: No Open Positions")
				m.statusMsg = errorStyle.Render("Flatten failed: No Open Positions")
				return m, nil
			}
			m.stopCurrentStrategy()

			if err := m.om.FlattenPositions(); err != nil {
				m.statusMsg = errorStyle.Render("Flatten failed: " + err.Error())
				return m, nil
			}
		}
		m.statusMsg = successStyle.Render("All positions flattened")
		m.mainLogger.Info("All positions flattened")
		m.orderLogger.Info("FLATTEN - All positions closed")

	case "mode":
		if len(parts) < 2 {
			m.statusMsg = errorStyle.Render("Usage: :mode <live|visual>")
			return m, nil
		}
		switch strings.ToLower(parts[1]) {
		case "l", "L":
			m.tradingMode = ModeLive
			m.statusMsg = successStyle.Render("Switched to LIVE mode")
			m.mainLogger.Info("Switched to LIVE trading mode")
		case "v", "V":
			m.tradingMode = ModeVisual
			m.statusMsg = "Switched to VISUAL mode"
			m.mainLogger.Info("Switched to VISUAL mode")
		default:
			m.statusMsg = errorStyle.Render("Invalid mode. Use l for'live' or v for 'visual'")
		}

	case "config":
		content, err := os.ReadFile(m.configPath)
		if err != nil {
			m.statusMsg = errorStyle.Render("Failed to read config: " + err.Error())
			return m, nil
		}
		m.isLogView = false
		m.editorTitle = "CONFIG EDITOR"
		m.configEditor.SetValue(string(content))
		m.configEditor.SetWidth(m.width - 4)
		m.configEditor.SetHeight(m.height - 10)
		m.mode = modeEditor
		m.statusMsg = "Editing config. Press Ctrl+S to save, ESC to exit, or : to run commands"
		return m, nil

	case "w", "write":
		if m.configEditor.Value() != "" && !m.isLogView {
			err := os.WriteFile(m.configPath, []byte(m.configEditor.Value()), 0644)
			if err != nil {
				m.statusMsg = errorStyle.Render("Failed to save config: " + err.Error())
			} else {
				m.statusMsg = successStyle.Render("Config saved")
			}
		}
		return m, nil

	case "x":
		// Save and exit
		if m.configEditor.Value() != "" && !m.isLogView {
			_ = os.WriteFile(m.configPath, []byte(m.configEditor.Value()), 0644)
		}
		m.mode = modeNormal
		m.isLogView = false
		m.statusMsg = "" // Reset status
		return m, nil

	case "export":
		if len(parts) < 2 {
			m.statusMsg = errorStyle.Render("Usage: :export <main|orders|strat>")
			return m, nil
		}
		logsDir := filepath.Join(config.GetProjectRoot(), "external", "logs")
		_ = os.MkdirAll(logsDir, 0755)
		switch parts[1] {
		case "main":
			content := m.mainLogger.ExportToString()
			filename := filepath.Join(logsDir, "main_log_"+time.Now().Format("January 2, 2006 3:04:05 PM")+".txt")
			err := os.WriteFile(filename, []byte(content), 0644)
			if err != nil {
				m.statusMsg = errorStyle.Render("Export failed: " + err.Error())
			} else {
				m.statusMsg = successStyle.Render("Log exported to " + filename)
				m.mainLogger.Printf("Main log exported to %s", filename)
			}
			return m, nil
		case "orders":
			content := m.orderLogger.ExportToString()
			filename := filepath.Join(logsDir, "orders_log_"+time.Now().Format("January 2, 2006 3:04:05 PM")+".txt")
			err := os.WriteFile(filename, []byte(content), 0644)
			if err != nil {
				m.statusMsg = errorStyle.Render("Export failed: " + err.Error())
			} else {
				m.statusMsg = successStyle.Render("Order log exported to " + filename)
				m.mainLogger.Printf("Order log exported to %s", filename)
			}
			return m, nil
		case "strat":
			content := m.strategyLogger.ExportToString()
			filename := filepath.Join(logsDir, "strat_log_"+time.Now().Format("January 2, 2006 3:04:05 PM")+".txt")
			err := os.WriteFile(filename, []byte(content), 0644)
			if err != nil {
				m.statusMsg = errorStyle.Render("Export failed: " + err.Error())
			} else {
				m.statusMsg = successStyle.Render("Order log exported to " + filename)
				m.mainLogger.Printf("Order log exported to %s", filename)
			}
			return m, nil
		default:
			m.statusMsg = errorStyle.Render("Invalid export target. Use 'log' or 'orders'")
		}

	case "help":
		m.activeTab = TabCommands
		m.statusMsg = "Switched to Commands"

	case "strategy":

		if m.currentStrategy != nil && m.currentStrategy.Runtime.Status() == StrategyRunning {
			m.statusMsg = errorStyle.Render("Cannot change strategy while running. Stop it first")
			return m, nil
		}
		if len(parts) < 2 {
			m.statusMsg = errorStyle.Render("Usage: :strategy <name>")
			return m, nil
		}
		stratName := parts[1]
		strat, err := execution.CreateStrategy(stratName, m.strategyLogger)
		if err != nil {
			m.statusMsg = errorStyle.Render("Failed to load strategy: " + err.Error())
			return m, nil
		}

		m.activeTab = TabStrategy
		m.scrollOffset = 0

		m.selectedStrategy = stratName
		m.currentStrategy = &StrategyState{
			Name:        strat.Name(),
			Params:      strat.GetParams(),
			Instance:    strat,
			Description: strat.Description(),
			Runtime:     &StrategyRuntime{},
		}

		// Reset params
		m.strategyParams = make(map[string]string)
		for _, p := range m.currentStrategy.Params {
			m.strategyParams[p.Name] = fmt.Sprintf("%v", p.Value)
		}

		m.strategyName = strat.Name()
		m.statusMsg = successStyle.Render("Loaded strategy: " + strat.Name())
		m.strategyLogger.Printf("Loaded strategy: %s", strat.Name())

	case "set":
		if m.currentStrategy != nil && m.currentStrategy.Runtime.Status() == StrategyRunning {
			m.statusMsg = errorStyle.Render("Cannot change parameters while strategy is running. Stop it first")
			return m, nil
		}
		if m.currentStrategy == nil {
			m.statusMsg = errorStyle.Render("No strategy selected. Use :strategy <name> first")
			return m, nil
		}
		if len(parts) < 3 {
			m.statusMsg = errorStyle.Render("Usage: :set <param> <value>")
			return m, nil
		}
		paramName := parts[1]
		paramValue := parts[2]

		found := false
		for _, p := range m.currentStrategy.Params {
			if p.Name == paramName {
				found = true
				break
			}
		}

		if !found {
			m.statusMsg = errorStyle.Render("Unknown parameter: " + paramName)
			return m, nil
		}

		m.strategyParams[paramName] = paramValue
		m.statusMsg = successStyle.Render(fmt.Sprintf("Set %s = %s", paramName, paramValue))
		m.strategyLogger.Printf("Parameter set: %s = %s", paramName, paramValue)

	case "start":
		if m.currentStrategy == nil {
			m.statusMsg = errorStyle.Render("No strategy selected")
			return m, nil
		}
		if m.om != nil && m.om.GetRiskManager().IsDailyLossExceeded(m.totalPnL) {
			m.statusMsg = errorStyle.Render("Cannot start strategy: daily loss limit exceeded")
			m.mainLogger.Error("Cannot start strategy: daily loss limit exceeded")
			return m, nil
		}
		if m.currentStrategy.Runtime.Status() == StrategyRunning {
			m.statusMsg = errorStyle.Render("Strategy is already running")
			return m, nil
		}
		if !m.connected {
			m.statusMsg = errorStyle.Render("Must be connected to start strategy")
			return m, nil
		}

		// Apply params
		for k, v := range m.strategyParams {
			if err := m.currentStrategy.Instance.SetParam(k, v); err != nil {
				m.statusMsg = errorStyle.Render("Failed to set param " + k + ": " + err.Error())
				return m, nil
			}
		}

		// Init strategy
		if err := m.currentStrategy.Instance.Init(m.om); err != nil {
			m.statusMsg = errorStyle.Render("Failed to initialize strategy: " + err.Error())
			return m, nil
		}

		m.currentStrategy.Symbol = m.strategyParams["symbol"]

		m.currentStrategy.Runtime.SetStatus(StrategyStarting)

		m.statusMsg = successStyle.Render("Strategy STARTED")
		m.strategyLogger.Info(">>> STRATEGY STARTED <<<")

		var lastBar LastBar

		var historicalLoaded bool

		m.marketDataSubscriptionManager.AddChartHandler(func(update marketdata.ChartUpdate) {
			m.strategyLogger.Debugf("CHART UPDATE RECEIVED at %s", time.Now().Format("15:04:05"))
			m.strategyLogger.Debugf("Chart handler called with %d charts", len(update.Charts))

			for _, chart := range update.Charts {
				m.strategyLogger.Debugf("Chart ID: %d | Bars: %d | EOH: %v",
					chart.ID, len(chart.Bars), chart.EOH)

				// Check for end of history marker
				if chart.EOH {
					m.strategyLogger.Debug("End of historical data - now receiving live updates")

					// Enable strategy for live trading
					if s, ok := m.currentStrategy.Instance.(interface{ SetEnabled(bool) }); ok {
						s.SetEnabled(true)
						m.strategyLogger.Info("Strategy enabled for LIVE trading")
						m.strategyLogger.Debug("tdsubs: ", m.tradingClientSubscriptionManager.GetActiveSubscriptions())
						m.strategyLogger.Debug("mdsubs: ", m.marketDataSubscriptionManager.GetActiveSubscriptions())
					}

					historicalLoaded = true
					continue
				}

				m.strategyLogger.Debugf("=== Chart ID: %d ===", chart.ID)
				m.strategyLogger.Debugf("Number of bars: %d", len(chart.Bars))

				if !historicalLoaded {
					// Process each bar
					for _, bar := range chart.Bars {
						// Skip only exact duplicates (same timestamp AND same close price)
						if bar.Timestamp == lastBar.Timestamp && bar.Close == lastBar.Close {
							continue
						}
						lastBar = LastBar{Timestamp: bar.Timestamp, Close: bar.Close}

						// Update Strategy
						if s, ok := m.currentStrategy.Instance.(interface {
							OnBar(string, float64) error
						}); ok {
							s.OnBar(bar.Timestamp, bar.Close)
						}

					}
				}
			}
		})

		m.strategyLogger.Debug("Chart Handler Added")

		var barAgg = &BarAggregator{firstTick: true}

		livebarcounter := 0

		m.marketDataSubscriptionManager.AddQuoteHandler(func(quote marketdata.Quote) {

			if !historicalLoaded {
				return
			}

			if trade, ok := quote.Entries["Trade"]; ok {
				price := trade.Price

				// Use the quote's actual timestamp, not time.Now()
				quoteTime, err := time.Parse(time.RFC3339, quote.Timestamp)
				if err != nil {
					return
				}

				currentMinute := quoteTime.UTC().Truncate(time.Minute).Format("2006-01-02T15:04Z")

				// New minute = new bar
				if currentMinute != barAgg.currentMinute {
					// Close previous bar if exists
					if !barAgg.firstTick {

						// Update Strategy
						if s, ok := m.currentStrategy.Instance.(interface {
							OnBar(string, float64) error
						}); ok {
							s.OnBar(barAgg.currentMinute, barAgg.close)
						}

						livebarcounter++
					}

					// Start new bar
					barAgg.currentMinute = currentMinute
					barAgg.open = price
					barAgg.high = price
					barAgg.low = price
					barAgg.close = price
					barAgg.firstTick = false
				} else {
					// Update current bar
					if price > barAgg.high {
						barAgg.high = price
					}
					if price < barAgg.low {
						barAgg.low = price
					}
					barAgg.close = price
				}
			}
		})

		m.strategyLogger.Debug("Quote Handler added")

		symbol := m.strategyParams["symbol"]
		go func() {
			m.marketDataSubscriptionManager.SubscribeQuote(symbol)

			mdparams := marketdata.HistoricalDataParams{
				Symbol: symbol,
				ChartDescription: marketdata.ChartDesc{
					UnderlyingType:  "MinuteBar",
					ElementSize:     1,
					ElementSizeUnit: "UnderlyingUnits",
				},
				TimeRange: marketdata.TimeRange{
					ClosestTimestamp: time.Now().Format(time.RFC3339),
					AsMuchAsElements: 25,
				},
			}

			err2 := m.marketDataSubscriptionManager.GetChart(mdparams)
			if err2 != nil {
				m.strategyLogger.Errorf("Failed to get chart: %v", err2)
			}

			m.currentStrategy.Runtime.SetStatus(StrategyRunning)

			m.strategyLogger.Debug("tdsubs: ", m.tradingClientSubscriptionManager.GetActiveSubscriptions())
			m.strategyLogger.Debug("mdsubs: ", m.marketDataSubscriptionManager.GetActiveSubscriptions())
		}()

	case "stop":

		m.stopCurrentStrategy()

	default:
		m.statusMsg = errorStyle.Render(fmt.Sprintf("Unknown command: %s", parts[0]))
	}

	return m, nil
}

func (m model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	// Tabs
	tabs := m.renderTabs()

	// Content area
	content := m.renderContent()

	// Status bar
	statusBar := m.renderStatusBar()

	// Command bar
	commandBar := m.renderCommandBar()

	// Combine everything
	return lipgloss.JoinVertical(
		lipgloss.Left,
		tabs,
		content,
		statusBar,
		commandBar,
	)
}

func (m model) renderTabs() string {
	tabs := []string{}

	tabNames := []string{"Main", "Strategy", "Order Mgmt", "Positions", "Commands"}
	for i, name := range tabNames {
		style := inactiveTabStyle
		if Tab(i) == m.activeTab {
			style = activeTabStyle
		}
		tabs = append(tabs, style.Render(fmt.Sprintf("%d:%s", i+1, name)))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (m model) renderContent() string {
	contentHeight := m.height - 5

	if m.mode == modeEditor {
		footer := "[Ctrl+S: Save | ESC: Cancel]"
		if m.isLogView {
			footer = "[Ctrl+E: Export to File | Ctrl+C: Copy | Ctrl+A: All | ESC: Exit]"
		}
		return contentStyle.
			Width(m.width - 4).
			Height(contentHeight).
			Render(fmt.Sprintf("═══ %s ═══\n\n%s\n\n%s", m.editorTitle, m.configEditor.View(), footer))
	}

	var content string
	switch m.activeTab {
	case TabMain:
		content = m.renderMainHub(contentHeight)
		return content
	case TabOrderManagement:
		content = m.renderOrderManagement(contentHeight)
		return content
	case TabPositions:
		content = m.renderPositions()
	case TabStrategy:
		content = m.renderStrategyTab(contentHeight)
		return content
	case TabCommands:
		content = m.renderCommandsScrollable(contentHeight)
		return contentStyle.Width(m.width - 4).Render(content)
	}

	return contentStyle.
		Width(m.width - 4).
		Height(contentHeight).
		Render(content)
}

func (m model) renderMainHub(contentHeight int) string {
	// Total available width
	availableWidth := m.width

	// Split 40/60 roughly, but ensure we fit
	leftWidth := (availableWidth * 4) / 10
	rightWidth := availableWidth - leftWidth

	// Adjust for borders/padding
	// contentStyle adds 2 horizontal padding, so let's respect that "safe area"
	// effectively we want the main hub to span the full width minus a small margin
	leftWidth -= 2
	rightWidth -= 2

	if leftWidth < 20 {
		leftWidth = 20
	}
	if rightWidth < 20 {
		rightWidth = 20
	}

	// Left panel - Menu
	var leftPanel strings.Builder

	// Header Section (Connection & Mode)
	connColor := "46" // Green
	connText := "CONNECTED"
	if !m.connected {
		connColor = "196" // Red
		connText = "DISCONNECTED"
	}

	modeText := "VISUAL"
	modeColor := "39" // Blue
	if m.tradingMode == ModeLive {
		modeText = "LIVE TRADING"
		modeColor = "196" // Red
	}

	header := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Foreground(lipgloss.Color(connColor)).Bold(true).Render("● "+connText),
		"  |  ",
		lipgloss.NewStyle().Foreground(lipgloss.Color(modeColor)).Bold(true).Render(modeText),
	)

	leftPanel.WriteString(header + "\n\n")
	leftPanel.WriteString(fmt.Sprintf("Strategy: %s\n", m.strategyName))
	leftPanel.WriteString(strings.Repeat("─", leftWidth-4) + "\n")

	// Menu Items (Compact)
	connectDesc := "Connect to API"
	if m.connected {
		connectDesc = "Disconnect from API"
	}

	menuItems := []struct {
		key  string
		desc string
	}{
		{"!", connectDesc},
		{"@", "Select Strategy"},
		{"#", "Export Log"},
		{"$", "Edit Config"},
		{"%", "Clear Log"},
	}

	for _, item := range menuItems {
		if item.key == "" {
			leftPanel.WriteString("\n")
			continue
		}

		// Check if command should be enabled
		enabled := true
		if !m.connected {
			// Only allow specific commands when disconnected
			// !, $, %, ^ are allowed
			switch item.key {
			case "!", "#", "$", "%":
				enabled = true
			default:
				enabled = false
			}
		}

		keyStyle := menuItemStyle
		descStyle := lipgloss.NewStyle()

		if !enabled {
			keyStyle = disabledStyle
			descStyle = disabledStyle
		}

		leftPanel.WriteString(fmt.Sprintf("%s %s\n",
			keyStyle.Width(3).Render("["+item.key+"]"),
			descStyle.Render(item.desc),
		))
	}

	leftContent := lipgloss.NewStyle().
		Width(leftWidth).
		Height(contentHeight).
		Padding(1).
		Render(leftPanel.String())

	// Right panel - Logger with scroll
	// Pass the exact calculation for width to avoid wrapping issues
	rightContent := m.renderLogPanel(rightWidth, contentHeight, "System Log", m.mainLogger, &m.logScrollOffset)

	// Combine panels
	return lipgloss.JoinHorizontal(lipgloss.Top, leftContent, rightContent)
}

func (m model) renderLogPanel(width, height int, title string, log *logger.Logger, scrollOffset *int) string {
	var logContent strings.Builder

	entries := log.GetEntries()

	// Layout Math:
	// Total Height = height
	// Border = 2 lines
	// Inner Height = height - 2
	// Title = 1 line
	// Separator = 1 line
	// Footer (Scroll status) = 1 line (effectively 2 with the newline separation)

	innerHeight := height - 2
	if innerHeight < 4 {
		innerHeight = 4
	} // Minimum safe height

	headerHeight := 2
	footerHeight := 2 // \n + text

	availableLines := innerHeight - headerHeight - footerHeight
	if availableLines < 1 {
		availableLines = 1
	}

	maxScroll := len(entries) - availableLines
	if maxScroll < 0 {
		maxScroll = 0
	}

	// Improved Auto-scroll logic:
	// If we were at the bottom (or very close), stay at the bottom even if many logs arrived.
	// We use a 2-line threshold to allow for slight offsets.
	if *scrollOffset >= maxScroll-2 {
		*scrollOffset = maxScroll
	}

	if *scrollOffset > maxScroll {
		*scrollOffset = maxScroll
	}
	if *scrollOffset < 0 {
		*scrollOffset = 0
	}

	// Get visible entries
	startIdx := *scrollOffset
	endIdx := *scrollOffset + availableLines
	if endIdx > len(entries) {
		endIdx = len(entries)
	}
	if startIdx >= len(entries) {
		startIdx = len(entries) - 1
		if startIdx < 0 {
			startIdx = 0
		}
	}

	visibleEntries := []logger.LogEntry{}
	if startIdx < len(entries) {
		visibleEntries = entries[startIdx:endIdx]
	}

	// Render log entries
	logWidth := width - 4 // Border (2) + Padding (2)
	if logWidth < 20 {
		logWidth = 20
	}

	for _, entry := range visibleEntries {
		timeStr := entry.Timestamp.Format("15:04:05")
		levelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

		switch entry.Level {
		case logger.LevelError:
			levelStyle = errorStyle
		case logger.LevelWarn:
			levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		case logger.LevelInfo:
			levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
		case logger.LevelDebug:
			levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
		}

		// Truncate message if too long
		msg := entry.Message
		// Time(8) + space + Level(5) + space + msg
		prefixLen := 15
		maxMsgLen := logWidth - prefixLen
		if maxMsgLen < 10 {
			maxMsgLen = 10
		}
		if len(msg) > maxMsgLen {
			msg = msg[:maxMsgLen-3] + "..."
		}

		logContent.WriteString(fmt.Sprintf("%s %s %s\n",
			timeStr,
			levelStyle.Render(fmt.Sprintf("%-5s", entry.Level)),
			msg))
	}

	// Fill empty lines to maintain height stability if not enough entries
	linesRendered := len(visibleEntries)
	if linesRendered < availableLines {
		logContent.WriteString(strings.Repeat("\n", availableLines-linesRendered))
	}

	// Add scroll indicator
	scrollPercent := 0.0
	if maxScroll > 0 {
		scrollPercent = float64(*scrollOffset) / float64(maxScroll) * 100
	}
	indicator := fmt.Sprintf("[%d/%d %.0f%%]", *scrollOffset+1, len(entries), scrollPercent)
	if len(entries) <= availableLines {
		indicator = "[All]"
	}

	logContent.WriteString("\n" + lipgloss.NewStyle().
		Foreground(lipgloss.Color("244")).
		Align(lipgloss.Right).
		Width(logWidth).
		Render(indicator))

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		Width(width - 4). // Inner width
		Align(lipgloss.Center)

	panel := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render(fmt.Sprintf("%s (%d)", title, len(entries))),
		strings.Repeat("─", width-4),
		logContent.String(),
	)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Width(width-2).      // width - border
		Height(innerHeight). // height - border
		Padding(0, 1).       // Padding for readability
		Render(panel)
}

func (m model) renderOrderManagement(contentHeight int) string {
	// Total available width
	availableWidth := m.width

	// Split 40/60
	leftWidth := (availableWidth * 4) / 10
	rightWidth := availableWidth - leftWidth

	// Adjust for borders/padding
	leftWidth -= 2
	rightWidth -= 2

	if leftWidth < 20 {
		leftWidth = 20
	}
	if rightWidth < 20 {
		rightWidth = 20
	}

	// Left panel - Metrics and Chart
	var leftPanel strings.Builder

	// Mode indicator
	modeText := "VISUAL MODE - Read Only"
	modeColor := "39"
	if m.tradingMode == ModeLive {
		modeText = "LIVE MODE - Trading Active"
		modeColor = "196"
	}
	leftPanel.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(modeColor)).
		Bold(true).
		Render(modeText))
	leftPanel.WriteString("\n\n")

	// Key metrics
	leftPanel.WriteString("═══ KEY METRICS ═══\n\n")

	pnlStyle := successStyle
	if m.totalPnL < 0 {
		pnlStyle = errorStyle
	}

	dailyRealizedStyle := successStyle
	if m.dailyrealizedPnL < 0 {
		dailyRealizedStyle = errorStyle
	}

	realizedStyle := successStyle
	if m.realizedPnL < 0 {
		realizedStyle = errorStyle
	}

	unrealizedStyle := successStyle
	if m.unrealizedPnL < 0 {
		unrealizedStyle = errorStyle
	}

	leftPanel.WriteString(fmt.Sprintf("%-22s %s\n", "Total P&L:", pnlStyle.Render(fmt.Sprintf("$%.2f", m.totalPnL))))
	leftPanel.WriteString(fmt.Sprintf("%-22s %s\n", "Daily Realized P&L:", dailyRealizedStyle.Render(fmt.Sprintf("$%.2f", m.dailyrealizedPnL))))
	leftPanel.WriteString(fmt.Sprintf("%-22s %s\n", "Session Realized P&L:", realizedStyle.Render(fmt.Sprintf("$%.2f", m.realizedPnL))))
	leftPanel.WriteString(fmt.Sprintf("%-22s %s\n", "Unrealized P&L:", unrealizedStyle.Render(fmt.Sprintf("$%.2f", m.unrealizedPnL))))
	leftPanel.WriteString("\n")
	leftPanel.WriteString(fmt.Sprintf("%-22s %d\n", "Open Positions:", len(m.positions)))

	if m.tradingMode == ModeLive {
		leftPanel.WriteString("\n\n═══ LIVE ACTIONS ═══\n\n")

		cmdStyle := menuItemStyle
		textStyle := lipgloss.NewStyle()
		if !m.connected {
			cmdStyle = disabledStyle
			textStyle = disabledStyle
		}

		leftPanel.WriteString(fmt.Sprintf("%-22s\n", cmdStyle.Render("Commands:")))
		leftPanel.WriteString(fmt.Sprintf("%-22s\n", textStyle.Render(":buy <symbol> <quantity>")))
		leftPanel.WriteString(fmt.Sprintf("%-22s\n", textStyle.Render(":sell <symbol> <quantity>")))
		leftPanel.WriteString(fmt.Sprintf("%-22s\n", textStyle.Render(":flatten")))

	}

	leftContent := lipgloss.NewStyle().
		Width(leftWidth).
		Height(contentHeight).
		Padding(1).
		Render(leftPanel.String())

	// Right panel - Order Logger
	rightContent := m.renderLogPanel(rightWidth, contentHeight, "Order Log", m.orderLogger, &m.orderLogScrollOffset)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftContent, rightContent)
}
func (m model) renderStrategyTab(contentHeight int) string {
	// Total available width
	availableWidth := m.width

	// Split 25/25/50
	leftWidth := (availableWidth * 25) / 100
	midWidth := (availableWidth * 25) / 100
	rightWidth := availableWidth - leftWidth - midWidth

	// Adjust for borders/padding
	leftWidth -= 2
	midWidth -= 2
	rightWidth -= 2

	if leftWidth < 20 {
		leftWidth = 20
	}
	if midWidth < 20 {
		midWidth = 20
	}
	if rightWidth < 20 {
		rightWidth = 20
	}

	// Left panel - Strategy Selection and Params
	var leftPanel strings.Builder
	leftPanel.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("═══ STRATEGY CONTROL ═══") + "\n\n")

	// Strategy Status

	statusColor := "196" // Red
	statusText := "INACTIVE"

	if m.currentStrategy != nil {
		switch m.currentStrategy.Runtime.Status() {
		case StrategyDisabled:
			statusColor = "196" // Red
			statusText = "INACTIVE"

		case StrategyStarting:
			statusColor = "214" // Orange
			statusText = "STARTING..."

		case StrategyRunning:
			statusColor = "46" // Green
			statusText = "RUNNING"

		case StrategyError:
			statusColor = "196" // Red
			statusText = "ERROR"

		case StrategyStopping:
			statusColor = "214" // Orange
			statusText = "STOPPING..."

		case StrategyStopped:
			statusColor = "196" // Red
			statusText = "INACTIVE"

		}
	}

	leftPanel.WriteString("Status: " + lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Bold(true).Render(statusText) + "\n\n")

	// Available Strategies
	leftPanel.WriteString(lipgloss.NewStyle().Bold(true).Render("Available Strategies:") + "\n")
	for _, s := range m.availableStrategies {
		prefix := "  "
		style := lipgloss.NewStyle()
		if s == m.selectedStrategy {
			prefix = "> "
			style = menuItemStyle
		}
		leftPanel.WriteString(style.Render(prefix+s) + "\n")
	}
	leftPanel.WriteString("\n")

	// Strategy Configuration
	if m.currentStrategy != nil {
		leftPanel.WriteString(lipgloss.NewStyle().Bold(true).Render("Configuration:") + "\n")
		for _, p := range m.currentStrategy.Params {
			val := m.strategyParams[p.Name]
			if val == "" {
				val = fmt.Sprintf("%v", p.Value)
			}
			leftPanel.WriteString(fmt.Sprintf("  %-12s: %s\n", p.Name, val))
		}
		leftPanel.WriteString("\n")

		leftPanel.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("Commands:") + "\n")
		leftPanel.WriteString("  :strategy <name>\n")
		leftPanel.WriteString("  :set <param> <val>\n")
		leftPanel.WriteString("  :start | :stop\n")
	}

	leftContent := lipgloss.NewStyle().
		Width(leftWidth).
		Height(contentHeight).
		Padding(1).
		Render(leftPanel.String())

	// Middle panel - Param View (Real-time Metrics)
	var midPanel strings.Builder
	midPanel.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("═══ PARAM VIEW ═══") + "\n\n")

	if m.currentStrategy != nil && m.currentStrategy.Runtime.Status() == StrategyRunning {
		metrics := m.currentStrategy.Instance.GetMetrics()
		if len(metrics) > 0 {
			// Sort keys for consistent display order
			keys := make([]string, 0, len(metrics))
			for k := range metrics {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			for _, name := range keys {
				val := metrics[name]
				midPanel.WriteString(fmt.Sprintf("%-12s: ", name))
				midPanel.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render(fmt.Sprintf("%.2f", val)) + "\n")
			}
		} else {
			midPanel.WriteString("Waiting for data...")
		}
	} else {
		midPanel.WriteString("Strategy not active")
	}

	midContent := lipgloss.NewStyle().
		Width(midWidth).
		Height(contentHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1).
		Render(midPanel.String())

	// Right panel - Strategy Logger
	rightContent := m.renderLogPanel(rightWidth, contentHeight, "Strategy Log", m.strategyLogger, &m.stratLogScrollOffset)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftContent, midContent, rightContent)
}
func (m model) renderPositions() string {
	if len(m.positions) == 0 {
		return "No positions"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-10s %8s %12s %12s\n", "Symbol", "Qty", "Avg Price", "P&L"))
	sb.WriteString(strings.Repeat("─", 50) + "\n")

	for _, pos := range m.positions {
		pnlStyle := successStyle
		if pos.PnL < 0 {
			pnlStyle = errorStyle
		}
		sb.WriteString(fmt.Sprintf("%-10s %8d %12.2f %s\n",
			pos.Symbol,
			pos.Quantity,
			pos.AvgPrice,
			pnlStyle.Render(fmt.Sprintf("$%.2f", pos.PnL)),
		))
	}

	return sb.String()
}

func (m model) renderCommandsContent() string {
	var sb strings.Builder
	// Search bar
	searchBarStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("255")).
		Padding(0, 1).
		Width(m.width - 6)

	searchPrompt := "Search: "
	if m.searchActive {
		sb.WriteString(searchBarStyle.Render(searchPrompt + m.searchInput + "█"))
	} else {
		sb.WriteString(searchBarStyle.Render(searchPrompt + m.searchInput + " (Press 'f' to search)"))
	}
	sb.WriteString("\n\n")

	// Filter commands based on search
	filteredCommands := m.commands
	if m.searchInput != "" {
		filteredCommands = []Command{}
		searchLower := strings.ToLower(m.searchInput)
		for _, cmd := range m.commands {
			if strings.Contains(strings.ToLower(cmd.Name), searchLower) ||
				strings.Contains(strings.ToLower(cmd.Description), searchLower) ||
				strings.Contains(strings.ToLower(cmd.Category), searchLower) {
				filteredCommands = append(filteredCommands, cmd)
			}
		}
	}

	if len(filteredCommands) == 0 {
		sb.WriteString("No commands found matching your search.\n")
		return sb.String()
	}

	// Group commands by category
	categoryMap := make(map[string][]Command)
	for _, cmd := range filteredCommands {
		categoryMap[cmd.Category] = append(categoryMap[cmd.Category], cmd)
	}

	// Render each category
	categories := []string{"Trading", "Navigation", "System"}
	for _, category := range categories {
		cmds, exists := categoryMap[category]
		if !exists || len(cmds) == 0 {
			continue
		}

		categoryStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			MarginTop(1).
			MarginBottom(1)

		sb.WriteString(categoryStyle.Render(fmt.Sprintf("═══ %s ═══", category)))
		sb.WriteString("\n")

		for _, cmd := range cmds {
			commandStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("46")).
				Bold(true)

			usageStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("244"))

			sb.WriteString(fmt.Sprintf("  %s\n", commandStyle.Render(cmd.Name)))
			sb.WriteString(fmt.Sprintf("    %s\n", cmd.Description))
			sb.WriteString(fmt.Sprintf("    %s\n", usageStyle.Render(cmd.Usage)))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
func (m model) renderCommandsScrollable(viewHeight int) string {
	// Render the full content
	fullContent := m.renderCommandsContent()
	// Split into lines
	lines := strings.Split(fullContent, "\n")

	// Calculate visible range
	totalLines := len(lines)
	visibleLines := viewHeight

	// Calculate max scroll - clamp immediately
	maxScroll := totalLines - visibleLines
	if maxScroll < 0 {
		maxScroll = 0
	}

	// Clamp scroll offset before using it
	scrollOffset := m.scrollOffset
	if scrollOffset > maxScroll {
		scrollOffset = maxScroll
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	// Get visible slice
	endLine := scrollOffset + visibleLines
	if endLine > totalLines {
		endLine = totalLines
	}

	visibleContent := strings.Join(lines[scrollOffset:endLine], "\n")

	// Add scroll indicator if there's more content
	scrollIndicator := ""
	if totalLines > visibleLines {
		scrollPercentage := float64(scrollOffset) / float64(maxScroll) * 100
		if maxScroll == 0 {
			scrollPercentage = 0
		}
		scrollIndicator = fmt.Sprintf("\n\n[Scroll: %d/%d lines (%.0f%%) - Use w/s or ↑/↓ to scroll, W=top, S=bottom]",
			scrollOffset+1, totalLines, scrollPercentage)
	}

	return visibleContent + scrollIndicator
}
func (m model) renderStatusBar() string {
	connStatus := "●"
	connColor := "46" // Green
	if !m.connected {
		connColor = "196" // Red
	}

	modeIndicator := ""
	if m.tradingMode == ModeLive {
		modeIndicator = errorStyle.Render(" [LIVE]")
	} else {
		modeIndicator = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render(" [VISUAL]")
	}

	left := fmt.Sprintf("%s Connected%s", lipgloss.NewStyle().Foreground(lipgloss.Color(connColor)).Render(connStatus), modeIndicator)

	// Calculate spacing safely to avoid negative repeat counts
	spacing := m.width - lipgloss.Width(left)
	if spacing < 1 {
		spacing = 1
	}
	statusText := left + strings.Repeat(" ", spacing)

	if m.statusMsg != "" {
		statusText = m.statusMsg
	}

	return statusBarStyle.Width(m.width).Render(statusText)
}
func (m model) renderCommandBar() string {
	var content string
	switch m.mode {
	case modeCommand:
		content = m.commandInput
	case modeEditor:
		content = "EDITOR: Type content, 'Ctrl+S' to save, 'ESC' to exit"
	default:
		switch {
		case m.activeTab == TabCommands && m.searchActive:
			content = "Searching... (Press ESC to exit search)"
		case m.activeTab == TabCommands:
			content = "w/s or ↑/↓ to scroll, W=top, S=bottom, f=search, :=command, q=quit"
		case m.activeTab == TabMain || m.activeTab == TabOrderManagement:
			content = "w/s to scroll logs, :=command, a/d or 1-5 to switch tabs, q=quit"
		default:
			content = "Press ':' for commands, 'q' to quit, 'a/d' or '1-5' to switch tabs"
		}
	}

	return commandBarStyle.Width(m.width).Render(content)
}

func (m model) stopCurrentStrategy() tea.Cmd {

	if m.currentStrategy == nil {
		m.statusMsg = errorStyle.Render("No strategy selected")
		return nil
	}

	if m.currentStrategy.Runtime.Status() != StrategyRunning {
		m.statusMsg = errorStyle.Render("Strategy is not running")
		return nil
	}
	m.currentStrategy.Runtime.SetStatus(StrategyStopping)

	// Reset strategy instance state so it can be re-initialized
	m.currentStrategy.Instance.Reset()
	m.currentStrategy.Runtime.SetStatus(StrategyStopped)

	m.statusMsg = successStyle.Render("Strategy STOPPED")
	m.strategyLogger.Info(">>> STRATEGY STOPPED <<<")
	return nil
}

func (m model) connectCmd() tea.Cmd {
	return func() tea.Msg {
		var cfg *config.Config
		var err error

		// Always reload config from disk to ensure latest values are used
		cfg, err = config.LoadOrCreateConfig(m.mainLogger)
		if err != nil {
			return connMsg{err: fmt.Errorf("config load error: %v", err)}
		}

		tm := auth.NewTokenManager(cfg)
		tm.SetLogger(m.mainLogger)

		m.mainLogger.Info("Attempting Authentication...")
		// Authenticate
		if err := tm.Authenticate(); err != nil {
			// ADD MORE CONTEXT HERE
			m.mainLogger.Errorf("Authentication failed: %v", err)
			return connMsg{err: fmt.Errorf("auth error: %w", err)}
		}

		sessionStart = time.Now().UTC()

		m.mainLogger.Infof("Session Start Time: %s", sessionStart)

		m.mainLogger.Info("Authentication Successful")

		om := execution.NewOrderManager(tm, cfg, m.orderLogger)

		var marketDataClient *tradovate.TradovateWebSocketClient
		var tradingClient *tradovate.TradovateWebSocketClient
		var marketDataSubscriptionManager *tradovate.DataSubscriber
		var tradingClientSubscriptionManager *tradovate.DataSubscriber

		//Get fresh tokens
		accessToken, err := tm.GetAccessToken()
		if err != nil {
			return connMsg{err: fmt.Errorf("failed to get access token: %w", err)}
		}
		m.mainLogger.Debug("Auth token aquired")

		tm.SetLogger(m.mainLogger)

		mdToken, err := tm.GetMDAccessToken()
		if err != nil {
			return connMsg{err: fmt.Errorf("failed to get MD token: %w", err)}
		}
		m.mainLogger.Debug("Market data token aquired")

		// Create new WebSocket clients
		marketDataClient = tradovate.NewTradovateWebSocketClient(mdToken, cfg.Tradovate.Environment, "md")
		marketDataClient.SetLogger(m.mainLogger)

		tradingClient = tradovate.NewTradovateWebSocketClient(accessToken, cfg.Tradovate.Environment, "")
		tradingClient.SetLogger(m.mainLogger)

		// Create subscription managers
		marketDataSubscriptionManager = tradovate.NewDataSubscriptionManager(marketDataClient)
		marketDataSubscriptionManager.SetLogger(m.strategyLogger)

		tradingClientSubscriptionManager = tradovate.NewDataSubscriptionManager(tradingClient)
		tradingClientSubscriptionManager.SetLogger(m.mainLogger)

		// Connect the clients
		if err := marketDataSubscriptionManager.Connect(); err != nil {
			return connMsg{err: fmt.Errorf("Error connecting market data client: %w", err)}
		}
		m.mainLogger.Info("Market Data WebSocket connected")

		if err := tradingClientSubscriptionManager.Connect(); err != nil {
			return connMsg{err: fmt.Errorf("Error connecting trading client: %w", err)}
		}
		m.mainLogger.Info("Trading WebSocket connected")

		// Set message handlers
		marketDataClient.SetMessageHandler(marketDataSubscriptionManager.HandleEvent)
		tradingClient.SetMessageHandler(tradingClientSubscriptionManager.HandleEvent)

		m.mainLogger.Debug("Message Handlers Set")

		//Set up order status handlers
		setupOrderHandlers := func() {
			tradingClientSubscriptionManager.OnOrderUpdate = func(data json.RawMessage) {
				var order struct {
					ID        int    `json:"id"`
					OrderType string `json:"orderType"`
					Action    string `json:"action"`
					OrdStatus string `json:"ordStatus"` // Note: Tradovate uses "ordStatus" not "orderStatus"
					Timestamp string `json:"timestamp"`
				}

				if err := json.Unmarshal(data, &order); err != nil {
					m.orderLogger.Warnf("Failed to parse order update: %v", err)
					return
				}

				orderTime, err := time.Parse(time.RFC3339Nano, order.Timestamp)
				if err != nil {
					m.orderLogger.Errorf("invalid order timestamp %q: %v", order.Timestamp, err)
				}

				if orderTime.Before(sessionStart) {
					return
				}

				ts := orderTime.Format("03:04:05 PM")

				switch order.OrdStatus {
				case "PendingNew":
					m.orderLogger.Infof("[%s UTC] ORDER PENDING  | ID=%d | %s %s",
						ts, order.ID, order.Action, order.OrderType)

				case "Filled":
					m.orderLogger.Infof("[%s UTC] ORDER FILLED   | ID=%d | %s %s",
						ts, order.ID, order.Action, order.OrderType)

				case "Rejected":
					m.orderLogger.Errorf("[%s UTC] ORDER REJECTED | ID=%d | %s %s",
						ts, order.ID, order.Action, order.OrderType)

				case "Working":
					m.orderLogger.Infof("[%s UTC] ORDER WORKING  | ID=%d | %s %s",
						ts, order.ID, order.Action, order.OrderType)
				case "Canceled":
					m.orderLogger.Infof("[%s UTC] ORDER CANCELED | ID=%d | %s %s",
						ts, order.ID, order.Action, order.OrderType)
				default:
					m.orderLogger.Warnf("[%s UTC] UNKNOWN ORDER STATUS | ID=%d | %s %s",
						ts, order.ID, order.Action, order.OrderType)
				}
			}
		}

		// Set up handlers initially
		setupOrderHandlers()
		m.mainLogger.Debug("OnOrderUpdate Set")

		userID := tm.GetUserID()
		tracker := portfolio.NewPortfolioTracker(tradingClientSubscriptionManager, marketDataSubscriptionManager, userID, m.mainLogger)

		if err := tracker.Start(cfg.Tradovate.Environment); err != nil {
			return connMsg{err: fmt.Errorf("Failed to start PortfolioTracker: %w", err)}
		}

		om.SetPortfolioTracker(tracker)

		tm.StartTokenRefreshMonitor(func() {
			m.mainLogger.Debug("Reconnection complete after token refresh")
		})

		return connMsgSuccess{
			config:            cfg,
			tokenManager:      tm,
			orderManager:      om,
			mdClient:          marketDataClient,
			mdSubscriber:      marketDataSubscriptionManager,
			tradingClient:     tradingClient,
			tradingSubscriber: tradingClientSubscriptionManager,
			portfolioTracker:  tracker,
		}
	}
}
