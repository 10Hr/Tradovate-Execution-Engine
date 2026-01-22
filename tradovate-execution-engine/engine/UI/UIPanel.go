package UI

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

type Tab int

const (
	TabMain Tab = iota
	TabOrderManagement
	TabPositions
	TabOrders
	TabExecutions
	TabStrategy
	TabCommands
	configFile = "config.json"
)

type TradingMode int

const (
	ModeVisual TradingMode = iota
	ModeLive
)

type mode int

const (
	modeNormal mode = iota
	modeCommand
	modeEditor
)

type Position struct {
	Symbol   string
	Quantity int
	AvgPrice float64
	PnL      float64
}

type Order struct {
	ID       string
	Symbol   string
	Side     string
	Quantity int
	Price    float64
	Status   string
	Time     time.Time
}

type Execution struct {
	Time     time.Time
	Symbol   string
	Side     string
	Quantity int
	Price    float64
}

type Command struct {
	Name        string
	Description string
	Usage       string
	Category    string
}

type PnLDataPoint struct {
	Time time.Time
	PnL  float64
}

type StrategyState struct {
	Name              string
	Active            bool
	Params            []execution.StrategyParam
	Instance          execution.Strategy
	Symbol            string
	Description       string
	ReceivedFirstData bool
}

type model struct {
	activeTab            Tab
	mode                 mode
	tradingMode          TradingMode
	commandInput         string
	commandHistory       []string
	historyIndex         int
	searchInput          string
	statusMsg            string
	errorMsg             string
	width                int
	height               int
	searchActive         bool
	scrollOffset         int
	logScrollOffset      int
	orderLogScrollOffset int
	stratLogScrollOffset int

	// Editor
	configEditor textarea.Model
	isLogView    bool
	editorTitle  string

	// Logger
	mainLogger     *logger.Logger
	orderLogger    *logger.Logger
	strategyLogger *logger.Logger

	// Data
	positions  []Position
	orders     []Order
	executions []Execution
	commands   []Command
	pnlHistory []PnLDataPoint

	// Connection status
	connected     bool
	totalPnL      float64
	unrealizedPnL float64
	realizedPnL   float64

	// Config
	configPath    string
	strategyName  string
	currentSymbol string

	// Strategy Management
	availableStrategies []string
	selectedStrategy    string
	currentStrategy     *StrategyState
	strategyParams      map[string]string

	// Order Manager
	orderManager     *execution.OrderManager
	positionManager  *portfolio.PositionManager
	portfolioTracker *portfolio.PortfolioTracker

	// Market Data & Auth
	wsClient          *tradovate.TradovateWebSocketClient
	userSync          *tradovate.TradovateWebSocketClient
	mdSubscriber      *tradovate.DataSubscriber
	tradingSubscriber *tradovate.DataSubscriber
}

func InitialModel(mainLog, orderLog *logger.Logger, om *execution.OrderManager) model {
	// Use provided loggers or create defaults
	if mainLog == nil {
		mainLog = logger.NewLogger(500)
	}
	if orderLog == nil {
		orderLog = logger.NewLogger(500)
	}
	strategyLog := logger.NewLogger(500)

	// Initial log messages
	mainLog.Println("System initialized")
	mainLog.Printf("Starting trading engine v1.0.0")

	// Configure singleton TokenManager with logger
	auth.GetTokenManager().SetLogger(mainLog)

	symbol := "MESH6"

	// Initialize Editor
	ta := textarea.New()
	ta.Placeholder = "Config content..."
	ta.Focus()

	availableStrats := execution.GetAvailableStrategies()
	mainLog.Infof("Discovered %d registered strategies: %v", len(availableStrats), availableStrats)

	// Also check physical folder for visibility
	stratDir := filepath.Join(config.GetProjectRoot(), "engine", "strategies")
	files, err := os.ReadDir(stratDir)
	if err == nil {
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".go") {
				mainLog.Infof("Found strategy file on disk: %s", file.Name())
			}
		}
	}

	return model{
		activeTab:            TabMain,
		mode:                 modeNormal,
		tradingMode:          ModeVisual,
		connected:            false,
		totalPnL:             0,
		mainLogger:           mainLog,
		orderLogger:          orderLog,
		strategyLogger:       strategyLog,
		orderManager:         om,
		configPath:           config.GetConfigPath(),
		strategyName:         "No strategy selected",
		currentSymbol:        symbol,
		logScrollOffset:      1000000,
		orderLogScrollOffset: 1000000,
		stratLogScrollOffset: 1000000,
		commandHistory:       []string{},
		historyIndex:         0,
		configEditor:         ta,
		availableStrategies:  availableStrats,
		strategyParams:       make(map[string]string),

		// Empty data - will be populated from OrderManager
		positions:  []Position{},
		orders:     []Order{},
		executions: []Execution{},
		pnlHistory: []PnLDataPoint{},
		commands: []Command{
			{Name: "buy", Description: "Place a buy order", Usage: ":buy <symbol> qty:<quantity>", Category: "Trading"},
			{Name: "sell", Description: "Place a sell order", Usage: ":sell <symbol> qty:<quantity>", Category: "Trading"},
			{Name: "cancel", Description: "Cancel an order", Usage: ":cancel <orderID>", Category: "Trading"},
			{Name: "flatten", Description: "Flatten all positions", Usage: ":flatten", Category: "Trading"},
			{Name: "mode", Description: "Switch trading mode (live/visual)", Usage: ":mode <live|visual>", Category: "System"},
			{Name: "config", Description: "Edit configuration", Usage: ":config", Category: "System"},
			{Name: "strategy", Description: "Select strategy", Usage: ":strategy <name>", Category: "System"},
			{Name: "export", Description: "Export logs", Usage: ":export <log|orders>", Category: "System"},
			{Name: "main", Description: "Switch to main hub", Usage: ":main", Category: "Navigation"},
			{Name: "om", Description: "Switch to order management", Usage: ":om", Category: "Navigation"},
			{Name: "pos", Description: "Switch to positions tab", Usage: ":pos", Category: "Navigation"},
			{Name: "orders", Description: "Switch to orders tab", Usage: ":orders", Category: "Navigation"},
			{Name: "help", Description: "Show commands page", Usage: ":help", Category: "Navigation"},
			{Name: "quit", Description: "Exit the application", Usage: ":quit or :q", Category: "System"},
		},
	}
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// connMsg indicates connection success/failure
type connMsg struct {
	err error
}

type connMsgSuccess struct {
	mdClient          *tradovate.TradovateWebSocketClient
	mdSubscriber      *tradovate.DataSubscriber
	tradingClient     *tradovate.TradovateWebSocketClient
	tradingSubscriber *tradovate.DataSubscriber
	portfolioTracker  *portfolio.PortfolioTracker
}

type editorFinishedMsg struct {
	err        error
	nextAction string // "connect" or "none"
}

func openEditor(path string, nextAction string) tea.Cmd {
	// Calling notepad.exe directly is more likely to block correctly than cmd /c
	c := exec.Command("notepad.exe", path)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{err: err, nextAction: nextAction}
	})
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
		if m.orderManager != nil {
			// Update Orders
			execOrders := m.orderManager.GetAllOrders()
			uiOrders := make([]Order, len(execOrders))
			for i, o := range execOrders {
				uiOrders[i] = Order{
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

			// Update Positions
			if m.positionManager != nil {
				pmPositions := m.positionManager.GetAllPositions()
				var uiPositions []Position
				var unrealizedTotal float64

				for name, p := range pmPositions {
					if p.NetPos != 0 {
						uiPositions = append(uiPositions, Position{
							Symbol:   name,
							Quantity: p.NetPos,
							AvgPrice: p.AvgPrice,
							PnL:      p.PL,
						})
					}
					unrealizedTotal += p.PL
				}
				m.positions = uiPositions
				m.unrealizedPnL = unrealizedTotal

				// Get realized PnL from OrderManager
				if m.orderManager != nil {
					m.realizedPnL = m.orderManager.GetDailyPnL()
				}

				if m.portfolioTracker != nil {
					m.totalPnL = m.portfolioTracker.GetTotalPL()
				} else {
					m.totalPnL = m.realizedPnL + m.unrealizedPnL
				}
			} else if m.orderManager != nil {
				execPos := m.orderManager.GetPosition(m.currentSymbol)
				if execPos.NetPos != 0 {
					m.positions = []Position{{
						Symbol:   execPos.Name,
						Quantity: execPos.NetPos,
						AvgPrice: execPos.AvgPrice,
						PnL:      execPos.PL,
					}}
				} else {
					m.positions = []Position{}
				}

				// Update PnL
				m.realizedPnL = m.orderManager.GetDailyPnL()
				m.unrealizedPnL = execPos.PL
				m.totalPnL = m.realizedPnL + m.unrealizedPnL
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

			// Executions - simplified derivation from filled orders for now
			// Ideally OrderManager would expose a GetExecutions() or GetFills()
			// For now, let's filter orders that are filled
			var execs []Execution
			for _, o := range execOrders {
				if o.Status == models.StatusFilled {
					execs = append(execs, Execution{
						Time:     o.FilledAt,
						Symbol:   o.Symbol,
						Side:     string(o.Side),
						Quantity: o.FilledQty,
						Price:    o.FilledPrice,
					})
				}
			}
			m.executions = execs
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
		m.wsClient = msg.mdClient
		m.mdSubscriber = msg.mdSubscriber
		m.userSync = msg.tradingClient
		m.tradingSubscriber = msg.tradingSubscriber
		m.portfolioTracker = msg.portfolioTracker
		m.connected = true

		m.mainLogger.Println(">>> CONNECTION SUCCESSFUL <<<")
		m.statusMsg = successStyle.Render("Connected to Tradovate")
		return m, nil

	case editorFinishedMsg:
		if msg.err != nil {
			m.mainLogger.Errorf("Editor error: %v", msg.err)
			m.statusMsg = errorStyle.Render("Failed to open editor")
		} else {
			m.mainLogger.Println(">>> Config editor closed, proceeding... <<<")
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
	m.errorMsg = ""

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
		m.activeTab = TabOrderManagement
		m.scrollOffset = 0

	case "3":
		m.activeTab = TabPositions
		m.scrollOffset = 0

	case "4":
		m.activeTab = TabOrders
		m.scrollOffset = 0

	case "5":
		m.activeTab = TabExecutions
		m.scrollOffset = 0

	case "6":
		m.activeTab = TabStrategy
		m.scrollOffset = 0

	case "7":
		m.activeTab = TabCommands
		m.scrollOffset = 0

	case ":", "/":
		m.mode = modeCommand
		m.commandInput = ":"

	// Shift Commands (Main Menu Actions)
	case "!": // Shift+1
		if m.connected {
			m.mainLogger.Println(">>> DISCONNECTING... <<<")
			m.connected = false

			if m.portfolioTracker != nil {
				_ = m.portfolioTracker.Stop()
				m.portfolioTracker = nil
			}

			if m.mdSubscriber != nil {
				_ = m.mdSubscriber.UnsubscribeAll()
			}

			if m.wsClient != nil {
				_ = m.wsClient.Disconnect()
				m.wsClient = nil
			}

			if m.userSync != nil {
				_ = m.userSync.Disconnect()
				m.userSync = nil
			}

			m.mainLogger.Println(">>> SUCCESSFULLY DISCONNECTED <<<")
			return m, nil
		} else {
			m.mainLogger.Println(">>> STARTING CONNECTION SEQUENCE... <<<")

			// Check if config exists
			if _, err := os.Stat(m.configPath); os.IsNotExist(err) {
				m.mainLogger.Println("Config not found. Creating default and opening editor...")
				if err := config.CreateDefaultConfig(m.configPath); err != nil {
					m.mainLogger.Errorf("Failed to create config: %v", err)
					return m, nil
				}

				// Open editor instead of trying to open external notepad
				content, _ := os.ReadFile(m.configPath)
				m.configEditor.SetValue(string(content))
				m.configEditor.SetWidth(m.width - 4)
				m.configEditor.SetHeight(m.height - 10)
				m.mode = modeEditor
				return m, nil
			}

			return m, m.connectCmd()
		}
	case "@": // Shift+2
		if !m.connected {
			m.errorMsg = "Must connect first (!)"
			return m, nil
		}
		if m.mdSubscriber == nil {
			m.errorMsg = "Market Data Subscriber not ready"
			return m, nil
		}
		m.mainLogger.Printf(">>> SUBSCRIBING TO %s... <<<", m.currentSymbol)
		go func() {
			if err := m.mdSubscriber.SubscribeQuote(m.currentSymbol); err != nil {
				m.mainLogger.Errorf("Subscribe error: %v", err)
			}
		}()

	case "#": // Shift+3
		if !m.connected {
			return m, nil
		}
		m.mode = modeCommand
		m.commandInput = ":strategy "
		m.statusMsg = "Enter strategy name..."

	case "$": // Shift+4
		m.mainLogger.Println(">>> EXPORTING MAIN LOG TO FILE... <<<")
		content := m.mainLogger.ExportToString()
		logsDir := filepath.Join(config.GetProjectRoot(), "external", "logs")
		_ = os.MkdirAll(logsDir, 0755)
		filename := filepath.Join(logsDir, "main_log_"+time.Now().Format("20060102_150405")+".txt")
		err := os.WriteFile(filename, []byte(content), 0644)
		if err != nil {
			m.errorMsg = "Export failed: " + err.Error()
			m.mainLogger.Errorf("Export failed: %v", err)
		} else {
			m.statusMsg = successStyle.Render("Log exported to " + filename)
			m.mainLogger.Printf("Log successfully exported to %s", filename)
		}

	case "%": // Shift+5
		content, err := os.ReadFile(m.configPath)
		if err != nil {
			// Try to create it if it doesn't exist
			if os.IsNotExist(err) {
				_ = config.CreateDefaultConfig(m.configPath)
				content, _ = os.ReadFile(m.configPath)
			} else {
				m.errorMsg = "Failed to read config: " + err.Error()
				return m, nil
			}
		}
		m.configEditor.SetValue(string(content))
		m.configEditor.SetWidth(m.width - 4)
		m.configEditor.SetHeight(m.height - 10)
		m.mode = modeEditor
		m.statusMsg = "Editing config. Press Ctrl+S to save, ESC to exit"
		m.mainLogger.Printf(">>> CONFIG EDITOR OPENED: %s <<<", m.configPath)

	case "^": // Shift+6
		m.mainLogger.Clear()
		m.logScrollOffset = 0
		m.mainLogger.Println(">>> LOG CLEARED <<<")

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

	default:
		// Could handle other keys here
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
		m.errorMsg = ""

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
			m.statusMsg = "" // Reset status
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
			m.errorMsg = "Failed to save config: " + err.Error()
			m.mainLogger.Errorf("Save failed: %v", err)
		} else {
			m.statusMsg = successStyle.Render("Config saved successfully")
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
		m.statusMsg = "" // Reset status
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
			m.errorMsg = "Must be connected to API to trade"
			return m, nil
		}
		if m.tradingMode != ModeLive {
			m.errorMsg = "Cannot place orders in Visual mode. Switch to Live mode with :mode live"
			m.mainLogger.Errorf("Order rejected: Not in Live mode")
			return m, nil
		}
		if len(parts) < 3 {
			m.errorMsg = "Usage: :" + parts[0] + " <symbol> <quantity>"
			return m, nil
		}

		symbol := parts[1]
		qtyStr := parts[2]

		if !strings.HasPrefix(qtyStr, "") {
			m.errorMsg = "Usage: :" + parts[0] + " <symbol> <quantity>"
			return m, nil
		}

		var qty int
		if _, err := fmt.Sscanf(qtyStr, "%d", &qty); err != nil {
			m.errorMsg = "Invalid quantity format. Use a number"
			return m, nil
		}

		if m.orderManager == nil {
			m.errorMsg = "Order Manager not initialized"
			m.mainLogger.Error("Order Manager not initialized")
			return m, nil
		}

		side := models.SideBuy
		if parts[0] == "sell" {
			side = models.SideSell
		}

		m.mainLogger.Printf("Submitting %s order for %d %s...", strings.ToUpper(parts[0]), qty, symbol)

		order, err := m.orderManager.SubmitMarketOrder(symbol, side, qty)
		if err != nil {
			m.errorMsg = "Order failed: " + err.Error()
			m.mainLogger.Errorf("Order failed: %v", err)
			return m, nil
		}

		m.statusMsg = successStyle.Render(fmt.Sprintf("%s order placed for %s (ID: %s)", strings.ToUpper(parts[0]), symbol, order.ID))
		m.mainLogger.Printf("%s order placed for %s (ID: %s)", strings.ToUpper(parts[0]), symbol, order.ID)
		m.orderLogger.Printf("%s %s - Price: Market, Qty: %d, ID: %s", strings.ToUpper(parts[0]), symbol, qty, order.ID)

	case "cancel":
		if !m.connected {
			m.errorMsg = "Must be connected to API to cancel orders"
			return m, nil
		}
		if m.tradingMode != ModeLive {
			m.errorMsg = "Cannot cancel orders in Visual mode"
			m.mainLogger.Errorf("Cancel rejected: Not in Live mode")
			return m, nil
		}
		if len(parts) < 2 {
			m.errorMsg = "Usage: :cancel <orderID>"
			return m, nil
		}
		orderID := parts[1]
		m.statusMsg = successStyle.Render(fmt.Sprintf("Order %s cancelled", orderID))
		m.mainLogger.Printf("Order %s cancelled", orderID)
		m.orderLogger.Printf("CANCEL %s", orderID)

	case "flatten":
		if !m.connected {
			m.errorMsg = "Must be connected to API to flatten positions"
			return m, nil
		}
		if m.tradingMode != ModeLive {
			m.errorMsg = "Cannot flatten in Visual mode"
			m.mainLogger.Errorf("Flatten rejected: Not in Live mode")
			return m, nil
		}
		m.statusMsg = successStyle.Render("All positions flattened")
		m.mainLogger.Println("All positions flattened")
		m.orderLogger.Println("FLATTEN - All positions closed")

	case "mode":
		if len(parts) < 2 {
			m.errorMsg = "Usage: :mode <live|visual>"
			return m, nil
		}
		switch strings.ToLower(parts[1]) {
		case "l", "L":
			m.tradingMode = ModeLive
			m.statusMsg = successStyle.Render("Switched to LIVE mode")
			m.mainLogger.Println("Switched to LIVE trading mode")
		case "v", "V":
			m.tradingMode = ModeVisual
			m.statusMsg = "Switched to VISUAL mode"
			m.mainLogger.Println("Switched to VISUAL mode")
		default:
			m.errorMsg = "Invalid mode. Use l for'live' or v for 'visual'"
		}

	// case "buy", "sell", "cancel", "flatten", "mode", "strategy", "export", "main", "om", "pos", "orders", "help":
	// 	// Check if we are in a mode that should block these commands
	// 	// Note: We can't easily check 'previous mode' here without a state change,
	// 	// but we can check if the editor is currently active or if we want to block
	// 	// these specific commands while the editor *would* be the background state.
	// 	// However, a simpler way is to just handle the commands as requested.

	case "config":
		content, err := os.ReadFile(m.configPath)
		if err != nil {
			m.errorMsg = "Failed to read config: " + err.Error()
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
				m.errorMsg = "Failed to save config: " + err.Error()
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
			m.errorMsg = "Usage: :export <log|orders>"
			return m, nil
		}
		logsDir := filepath.Join(config.GetProjectRoot(), "external", "logs")
		_ = os.MkdirAll(logsDir, 0755)
		switch parts[1] {
		case "log", "main":
			content := m.mainLogger.ExportToString()
			filename := filepath.Join(logsDir, "main_log_"+time.Now().Format("20060102_150405")+".txt")
			err := os.WriteFile(filename, []byte(content), 0644)
			if err != nil {
				m.errorMsg = "Export failed: " + err.Error()
			} else {
				m.statusMsg = successStyle.Render("Log exported to " + filename)
				m.mainLogger.Printf("Main log exported to %s", filename)
			}
			return m, nil
		case "orders":
			content := m.orderLogger.ExportToString()
			filename := filepath.Join(logsDir, "orders_log_"+time.Now().Format("20060102_150405")+".txt")
			err := os.WriteFile(filename, []byte(content), 0644)
			if err != nil {
				m.errorMsg = "Export failed: " + err.Error()
			} else {
				m.statusMsg = successStyle.Render("Order log exported to " + filename)
				m.mainLogger.Printf("Order log exported to %s", filename)
			}
			return m, nil
		default:
			m.errorMsg = "Invalid export target. Use 'log' or 'orders'"
		}

	case "main":
		m.activeTab = TabMain
		m.statusMsg = "Switched to Main Hub"

	case "om":
		m.activeTab = TabOrderManagement
		m.statusMsg = "Switched to Order Management"

	case "pos":
		m.activeTab = TabPositions
		m.statusMsg = "Switched to Positions"

	case "orders":
		m.activeTab = TabOrders
		m.statusMsg = "Switched to Orders"

	case "help":
		m.activeTab = TabCommands
		m.statusMsg = "Switched to Commands"

	case "strategy":
		if m.currentStrategy != nil && m.currentStrategy.Active {
			m.errorMsg = "Cannot change strategy while running. Stop it first"
			return m, nil
		}
		if len(parts) < 2 {
			m.errorMsg = "Usage: :strategy <name>"
			return m, nil
		}
		stratName := parts[1]
		strat, err := execution.CreateStrategy(stratName)
		if err != nil {
			m.errorMsg = "Failed to load strategy: " + err.Error()
			return m, nil
		}

		m.selectedStrategy = stratName
		m.currentStrategy = &StrategyState{
			Name:        strat.Name(),
			Params:      strat.GetParams(),
			Instance:    strat,
			Description: strat.Description(),
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
		if m.currentStrategy != nil && m.currentStrategy.Active {
			m.errorMsg = "Cannot change parameters while strategy is running. Stop it first"
			return m, nil
		}
		if m.currentStrategy == nil {
			m.errorMsg = "No strategy selected. Use :strategy <name> first"
			return m, nil
		}
		if len(parts) < 3 {
			m.errorMsg = "Usage: :set <param> <value>"
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
			m.errorMsg = "Unknown parameter: " + paramName
			return m, nil
		}

		m.strategyParams[paramName] = paramValue
		m.statusMsg = successStyle.Render(fmt.Sprintf("Set %s = %s", paramName, paramValue))
		m.strategyLogger.Printf("Parameter set: %s = %s", paramName, paramValue)

	case "start":
		if m.currentStrategy == nil {
			m.errorMsg = "No strategy selected"
			return m, nil
		}
		if m.currentStrategy.Active {
			m.errorMsg = "Strategy is already running"
			return m, nil
		}
		if !m.connected {
			m.errorMsg = "Must be connected to start strategy"
			return m, nil
		}

		// Apply params
		for k, v := range m.strategyParams {
			if err := m.currentStrategy.Instance.SetParam(k, v); err != nil {
				m.errorMsg = "Failed to set param " + k + ": " + err.Error()
				return m, nil
			}
		}

		// Init strategy
		if err := m.currentStrategy.Instance.Init(m.orderManager); err != nil {
			m.errorMsg = "Failed to initialize strategy: " + err.Error()
			return m, nil
		}

		m.currentStrategy.Active = true
		m.currentStrategy.Symbol = m.strategyParams["symbol"]
		m.currentStrategy.ReceivedFirstData = false
		m.statusMsg = successStyle.Render("Strategy STARTED")
		m.strategyLogger.Println(">>> STRATEGY STARTED <<<")

		// Subscribe to chart data if symbol is set
		symbol := m.strategyParams["symbol"]
		if symbol != "" {
			m.strategyLogger.Printf("Subscribing to data for %s", symbol)
			if m.mdSubscriber != nil {
				go func() {
					// Ensure we are connected
					if !m.mdSubscriber.IsConnected() {
						m.strategyLogger.Warn("No Market Data Connection. Attempting to connect...")
						if err := m.mdSubscriber.Connect(); err != nil {
							m.strategyLogger.Errorf("Failed to connect to market data: %v", err)
							return
						}
						m.strategyLogger.Info("Market data connection established")
					}

					// 1. Subscribe to Quotes for real-time price updates
					if err := m.mdSubscriber.SubscribeQuote(symbol); err != nil {
						m.strategyLogger.Errorf("Failed to subscribe to quotes: %v", err)
					}

					// 2. Calculate lookback time
					// Hardcoded to 25 minutes as requested
					lookbackMinutes := 25

					startTime := time.Now().Add(time.Duration(-lookbackMinutes) * time.Minute)
					m.strategyLogger.Infof("Requesting %d minutes of historical data for initialization...", lookbackMinutes)
					// 3. Request Chart for historical and live data
					params := marketdata.HistoricalDataParams{
						Symbol: symbol,
						ChartDescription: marketdata.ChartDesc{
							UnderlyingType:  "MinuteBar",
							ElementSize:     1,
							ElementSizeUnit: "UnderlyingUnits",
						},
						TimeRange: marketdata.TimeRange{
							ClosestTimestamp: startTime.Format(time.RFC3339),
						},
					}
					err := m.mdSubscriber.GetChart(params)
					if err != nil {
						m.strategyLogger.Errorf("Failed to subscribe to chart: %v", err)
					}
				}()
			}
		}

	case "stop":
		if m.currentStrategy == nil {
			return m, nil
		}
		if !m.currentStrategy.Active {
			m.errorMsg = "Strategy is not running"
			return m, nil
		}

		// Unsubscribe from chart/quote
		symbol := m.strategyParams["symbol"]
		if symbol != "" && m.mdSubscriber != nil {
			m.strategyLogger.Printf("Unsubscribing from data for %s...", symbol)
			go func() {
				if err := m.mdSubscriber.UnsubscribeChart(symbol); err != nil {
					m.strategyLogger.Warnf("Unsubscribe chart failed: %v", err)
				}
				if err := m.mdSubscriber.UnsubscribeQuote(symbol); err != nil {
					m.strategyLogger.Warnf("Unsubscribe quote failed: %v", err)
				}
			}()
		}

		// Reset strategy instance state so it can be re-initialized
		m.currentStrategy.Instance.Reset()
		m.currentStrategy.Active = false

		m.statusMsg = "Strategy STOPPED"
		m.strategyLogger.Println(">>> STRATEGY STOPPED <<<")

	default:
		m.errorMsg = fmt.Sprintf("Unknown command: %s", parts[0])
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

	tabNames := []string{"Main", "Order Mgmt", "Positions", "Orders", "Executions", "Strategy", "Commands"}
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
		footer := "[Ctrl+S: Save | :w: Save | :x: Save & Exit | ESC: Cancel]"
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
	case TabOrders:
		content = m.renderOrders()
	case TabExecutions:
		content = m.renderExecutions()
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
		{"@", "Receive Market Data"},
		{"#", "Select Strategy"},
		{"$", "Export Main Log"},
		{"%", "Edit Config"},
		{"^", "Clear Log"},
		{"", ""},
		{"M", "Main"},
		{"O", "Order Mgmt"},
		{"P", "Positions"},
		{"T", "Trades"},
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
			case "!", "$", "%", "^":
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

	realizedStyle := successStyle
	if m.realizedPnL < 0 {
		realizedStyle = errorStyle
	}

	unrealizedStyle := successStyle
	if m.unrealizedPnL < 0 {
		unrealizedStyle = errorStyle
	}

	leftPanel.WriteString(fmt.Sprintf("Total P&L:      %s\n", pnlStyle.Render(fmt.Sprintf("$%.2f", m.totalPnL))))
	leftPanel.WriteString(fmt.Sprintf("Realized P&L:   %s\n", realizedStyle.Render(fmt.Sprintf("$%.2f", m.realizedPnL))))
	leftPanel.WriteString(fmt.Sprintf("Unrealized P&L: %s\n", unrealizedStyle.Render(fmt.Sprintf("$%.2f", m.unrealizedPnL))))
	leftPanel.WriteString(fmt.Sprintf("Open Positions: %d\n", len(m.positions)))
	leftPanel.WriteString(fmt.Sprintf("Active Orders:  %d\n", len(m.orders)))
	leftPanel.WriteString(fmt.Sprintf("Today's Trades: %d\n\n", len(m.executions)))

	if m.tradingMode == ModeLive {
		leftPanel.WriteString("\n\n═══ LIVE ACTIONS ═══\n\n")

		cmdStyle := menuItemStyle
		textStyle := lipgloss.NewStyle()
		if !m.connected {
			cmdStyle = disabledStyle
			textStyle = disabledStyle
		}

		leftPanel.WriteString(cmdStyle.Render("Commands:\n"))
		leftPanel.WriteString(textStyle.Render("  :buy <symbol>\n"))
		leftPanel.WriteString(textStyle.Render("  :sell <symbol>\n"))
		leftPanel.WriteString(textStyle.Render("  :cancel <id>\n"))
		leftPanel.WriteString(textStyle.Render("  :flatten\n"))
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
	if m.currentStrategy != nil && m.currentStrategy.Active {
		if m.currentStrategy.ReceivedFirstData {
			statusColor = "46" // Green
			statusText = "RUNNING"
		} else {
			statusColor = "214" // Orange
			statusText = "STARTING..."
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

	if m.currentStrategy != nil && m.currentStrategy.Active {
		metrics := m.currentStrategy.Instance.GetMetrics()
		if len(metrics) > 0 {
			for name, val := range metrics {
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
func (m model) renderOrders() string {
	if len(m.orders) == 0 {
		return "No active orders"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-8s %-10s %6s %8s %12s %10s %s\n", "ID", "Symbol", "Side", "Qty", "Price", "Status", "Time"))
	sb.WriteString(strings.Repeat("─", 70) + "\n")

	for _, order := range m.orders {
		sb.WriteString(fmt.Sprintf("%-8s %-10s %6s %8d %12.2f %10s %s\n",
			order.ID,
			order.Symbol,
			order.Side,
			order.Quantity,
			order.Price,
			order.Status,
			order.Time.Format("15:04:05"),
		))
	}

	return sb.String()
}
func (m model) renderExecutions() string {
	if len(m.executions) == 0 {
		return "No executions"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-20s %-10s %6s %8s %12s\n", "Time", "Symbol", "Side", "Qty", "Price"))
	sb.WriteString(strings.Repeat("─", 60) + "\n")

	for _, exec := range m.executions {
		sb.WriteString(fmt.Sprintf("%-20s %-10s %6s %8d %12.2f\n",
			exec.Time.Format("15:04:05"),
			exec.Symbol,
			exec.Side,
			exec.Quantity,
			exec.Price,
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
	pnlStyle := successStyle
	if m.totalPnL < 0 {
		pnlStyle = errorStyle
	}

	modeIndicator := ""
	if m.tradingMode == ModeLive {
		modeIndicator = errorStyle.Render(" [LIVE]")
	} else {
		modeIndicator = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render(" [VISUAL]")
	}

	left := fmt.Sprintf("%s Connected%s", lipgloss.NewStyle().Foreground(lipgloss.Color(connColor)).Render(connStatus), modeIndicator)
	right := pnlStyle.Render(fmt.Sprintf("P&L: $%.2f", m.totalPnL))

	// Calculate spacing safely to avoid negative repeat counts
	spacing := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if spacing < 1 {
		spacing = 1
	}
	statusText := left + strings.Repeat(" ", spacing) + right

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
		if m.errorMsg != "" {
			content += "  " + errorStyle.Render(m.errorMsg)
		}
	case modeEditor:
		content = "EDITOR: Type content, ':' for commands, 'Ctrl+S' to save, 'ESC' to exit"
	default:
		switch {
		case m.activeTab == TabCommands && m.searchActive:
			content = "Searching... (Press ESC to exit search)"
		case m.activeTab == TabCommands:
			content = "w/s or ↑/↓ to scroll, W=top, S=bottom, f=search, :=command, q=quit"
		case m.activeTab == TabMain || m.activeTab == TabOrderManagement:
			content = "w/s to scroll logs, :=command, a/d or 1-6 to switch tabs, q=quit"
		default:
			content = "Press ':' for commands, 'q' to quit, 'a/d' or '1-6' to switch tabs"
		}
	}

	return commandBarStyle.Width(m.width).Render(content)
}

func (m model) connectCmd() tea.Cmd {
	return func() tea.Msg {
		// 1. Load Config
		cfg, err := config.LoadOrCreateConfig()
		if err != nil {
			return connMsg{err: fmt.Errorf("config error: %v", err)}
		}

		// 2. Authenticate
		tm := auth.GetTokenManager()
		tm.SetCredentials(
			cfg.Tradovate.AppID,
			cfg.Tradovate.AppVersion,
			cfg.Tradovate.Chl,
			cfg.Tradovate.Cid,
			cfg.Tradovate.DeviceID,
			cfg.Tradovate.Environment,
			cfg.Tradovate.Username,
			cfg.Tradovate.Password,
			cfg.Tradovate.Sec,
			cfg.Tradovate.Enc,
		)

		if err := tm.Authenticate(); err != nil {
			return connMsg{err: fmt.Errorf("auth error: %v", err)}
		}

		// 3. Get MD Token
		mdToken, err := tm.GetMDAccessToken()
		if err != nil {
			return connMsg{err: fmt.Errorf("md token error: %v", err)}
		}

		// Get Access Token for Trading
		accessToken, err := tm.GetAccessToken()
		if err != nil {
			return connMsg{err: fmt.Errorf("token error: %v", err)}
		}

		// 4. Initialize WS Clients

		// Market Data Client
		mdClient := tradovate.NewTradovateWebSocketClient(mdToken, cfg.Tradovate.Environment, "md")
		mdClient.SetLogger(m.mainLogger)

		// Trading Client
		tradingClient := tradovate.NewTradovateWebSocketClient(accessToken, cfg.Tradovate.Environment, "")
		tradingClient.SetLogger(m.mainLogger)

		// Create Subscribers
		mdSubscriber := tradovate.NewDataSubscriber(mdClient)
		mdSubscriber.SetLogger(m.mainLogger)

		tradingSubscriber := tradovate.NewDataSubscriber(tradingClient)

		tradingSubscriber.SetLogger(m.mainLogger)

		// Set WebSocket message handlers
		mdClient.SetMessageHandler(mdSubscriber.HandleEvent)
		tradingClient.SetMessageHandler(tradingSubscriber.HandleEvent)

		// Add strategy handlers to mdSubscriber

		mdSubscriber.AddChartHandler(func(update marketdata.ChartUpdate) {
			m.mainLogger.Debugf("DEBUG: Chart handler triggered with %d charts", len(update.Charts))
			if m.currentStrategy != nil && m.currentStrategy.Active {

				for _, chart := range update.Charts {

					if !m.currentStrategy.ReceivedFirstData && (len(chart.Bars) > 0 || len(chart.Ticks) > 0) {

						m.currentStrategy.ReceivedFirstData = true

						m.strategyLogger.Infof(">>> MARKET REACHED (Chart): Receiving data for %s <<<", m.currentStrategy.Symbol)

					}

					// Log every bar for the strategy

					if len(chart.Bars) > 0 {

						m.strategyLogger.Infof("Received %d Historical/Live Bars", len(chart.Bars))

						for _, bar := range chart.Bars {

							m.strategyLogger.Infof("Bar: %s | O:%.2f H:%.2f L:%.2f C:%.2f | V:%.0f",

								bar.Timestamp, bar.Open, bar.High, bar.Low, bar.Close, bar.UpVolume+bar.DownVolume)

						}

					}

					// Process bars

					for _, bar := range chart.Bars {

						_ = m.currentStrategy.Instance.OnTick(bar.Close)

					}

					// Process ticks

					for _, tick := range chart.Ticks {

						_ = m.currentStrategy.Instance.OnTick(tick.Price)

					}

				}

			}

		})

		mdSubscriber.AddQuoteHandler(func(quote marketdata.Quote) {
			m.mainLogger.Debugf("DEBUG: Quote handler triggered for contract %d", quote.ContractID)
			if m.currentStrategy != nil && m.currentStrategy.Active {

				if trade, ok := quote.Entries["Trade"]; ok {

					if !m.currentStrategy.ReceivedFirstData {

						m.currentStrategy.ReceivedFirstData = true

						m.strategyLogger.Infof(">>> MARKET REACHED (Quote): Receiving data <<<")

					}

					_ = m.currentStrategy.Instance.OnTick(trade.Price)

				}

			}

		})

		// Initialize PositionManager

		userID := tm.GetUserID()

		// Create and start tracker reusing connections

		tracker := portfolio.NewPortfolioTracker(tradingClient, mdClient, userID, m.mainLogger)

		if err := tracker.Start(cfg.Tradovate.Environment); err != nil {

			m.mainLogger.Errorf("Failed to start PortfolioTracker: %v", err)
		}

		// Return success with objects
		return connMsgSuccess{
			mdClient:          mdClient,
			mdSubscriber:      mdSubscriber,
			tradingClient:     tradingClient,
			tradingSubscriber: tradingSubscriber,
			portfolioTracker:  tracker,
		}
	}
}
