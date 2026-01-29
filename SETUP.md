# Tradovate Execution Engine - Setup Guide

## Prerequisites

### Required Software
1. **Go 1.25.6 or higher** - [Download](https://go.dev/dl/)
2. **Terminal** - Minimum 80x24 characters (larger recommended)
3. **Internet connection** for API access

### Required Accounts
1. **MyFundedFutures Account** - [Create Account](https://myfundedfutures.com)
2. **Tradovate Credentials** - Obtained from Tradovate platform

## Installation Steps

### 1. Clone the Repository

```bash
git clone https://github.com/10Hr/Tradovate-Execution-Engine.git
cd Tradovate-Execution-Engine/tradovate-execution-engine
```

### 2. Install Dependencies

The project uses Go modules. Install dependencies with:

```bash
go mod download
```

**Core Dependencies** (from `go.mod` lines 5-11):
```
github.com/atotto/clipboard v0.1.4
github.com/charmbracelet/bubbles v0.21.0
github.com/charmbracelet/bubbletea v1.3.10
github.com/charmbracelet/lipgloss v1.1.0
github.com/gorilla/websocket v1.5.3
```

### 3. Build the Application

From the `tradovate-execution-engine` directory:

```bash
go build -o trading-engine.exe ./engine/cmd/main.go
```

This creates an executable named trading-engine.exe.


### 4. First Run

Execute the application:

```bash
./trading-engine.exe
```

**What happens on first run** (from `config.go` lines 52-65):
1. Tests run automatically (see `cmd/main.go` line 14) and are exported to `tests/tests.txt`
2. Launches TUI interface
3. Application looks for `config/config.json`
4. If not found, creates default configuration file
5. You'll see a message: "Config file not found... Creating default config"
6. You must configure credentials before connecting

## Configuration

### Configuration File Location

The config file will be created at:
```
tradovate-execution-engine/config/config.json
```

From `config.go` lines 46-50:
```go
// GetConfigPath returns the absolute path to the config file
func GetConfigPath() string {
	rootDir := GetProjectRoot()
	return filepath.Join(rootDir, "config", "config.json")
}
```

### Default Configuration Structure

From `config.go` lines 129-151, the default config created is:

```json
{
  "tradovate": {
    "appId": "your_app_id_here",
    "appVersion": "your_app_version_here",
    "chl": "your_chl_here",
    "cid": "your_cid_here",
    "deviceId": "your_device_id_here",
    "environment": "'live' or 'demo'",
    "username": "your_username_here",
    "password": "your_password_here",
    "sec": "your_security_token_here",
    "enc": true
  },
  "risk": {
    "maxContracts": 1,
    "dailyLossLimit": 500.0,
    "enableRiskChecks": true
  }
}
```

### Obtaining Tradovate Credentials

**For detailed instructions on extracting Tradovate API credentials, see:**
[Credential Extraction Guide](https://docs.google.com/document/d/16pkRcAYS2NYf8K-AFAIOAEIwCmt5l2MzJlOkV66RcIs/edit?usp=sharing)

This guide provides step-by-step instructions for obtaining:
- `appId`
- `appVersion`
- `chl`
- `cid`
- `deviceId`
- `sec` (security token)
- `username` and `password`

**⚠️ Important Notes:**
- Demo credentials only work with `"environment": "demo"`
- Shared demo account may have conflicts with multiple users

### Environment Settings

From `config/types.go` lines 14-25, the configuration supports two environments:

**Demo Environment:**
```json
"environment": "demo"
```
- Uses Tradovate demo/simulation servers
- No real money at risk
- Ideal for testing
- Uses endpoints from `config.go` lines 13, 18

**Live Environment:**
```json
"environment": "live"
```
**(UNTESTED)**
- Connects to live Tradovate trading servers 
- ⚠️ **REAL MONEY TRADING**
- Requires live trading account
- Uses endpoints from `config.go` lines 12, 17

### Editing Configuration

**Method 1: Built-in Editor**

*Note*: You can not use Ctrl cmds insude the editor Ex: `Ctrl + C`. 

1. Launch the application
2. Press `Shift + 4` (the `$` key) OR type `:config` and press Enter
3. Edit the configuration in the integrated text editor
4. Press `Ctrl + S` to save
5. Press `Esc` to exit the editor
6. Look for "Config saved via integrated editor" in the System Log

**Method 2: External Editor**

1. Close the application if running
2. Edit `tradovate-execution-engine/config/config.json` in your preferred text editor
3. Save the file
4. Restart the application

### Risk Configuration

From `config/types.go` lines 27-32:

```go
// RiskConfig holds risk management and order configuration
type RiskConfig struct {
	MaxContracts     int     `json:"maxContracts"`
	DailyLossLimit   float64 `json:"dailyLossLimit"`
	EnableRiskChecks bool    `json:"enableRiskChecks"`
}
```

**maxContracts:**
- Maximum position size allowed
- Default: `1`
- User adjustable
- Note: Built-in MA Crossover strategy is hardcoded to 1 contract per trade

**dailyLossLimit:**
- Maximum allowed loss per trading day (in dollars)
- Default: `500.0`
- User adjustable
- Triggers automatic position flattening and strategy stop when breached

**enableRiskChecks:**
- Set to `true`: Enable all risk checks (recommended)
- Set to `false`: Disable risk checks (⚠️ NOT RECOMMENDED)

## Connecting to Tradovate

### First Connection

1. Ensure configuration file is properly filled out
2. Launch the application
3. Press `Shift + 1` (the `!` key) to connect
4. Wait for connection confirmation in System Log

**Expected log messages (from `websocket_manager.go` lines 49-62):**
```
"Connecting to WebSocket: <url>"
"WebSocket connected"
"Sending authorization..."
"WebSocket authorized"
```

### Connection Issues

If connection fails, check:

1. **Configuration File**
   - All fields are filled out (no "your_xxx_here" placeholders)
   - Environment matches account type (`demo` for demo accounts, `live` for live accounts)
   - Credentials are current and valid

2. **Network**
   - Internet connection is active
   - Firewall isn't blocking WebSocket connections (ports 443/80)
   - No proxy blocking WebSocket protocols

3. **Tradovate API Status**
   - Verify Tradovate services are online
   - Check for maintenance windows

## Project Structure

After installation, your directory structure will be:

```
Tradovate-Execution-Engine/
└── tradovate-execution-engine/
    ├── engine/
    │   ├── cmd/
    │   │   └── main.go                # Entry point
    │   ├── config/
    │   │   ├── config.go              # Config management
    │   │   └── types.go               # Config structures
    │   ├── indicators/
    │   │   └── SMA.go                 # SMA indicator
    │   ├── internal/
    │   │   ├── auth/                  # Authentication
    │   │   │   ├── token_manager.go
    │   │   │   └── types.go
    │   │   ├── execution/             # Order & strategy mgmt
    │   │   │   ├── order_manager.go
    │   │   │   ├── strategy_manager.go
    │   │   │   └── types.go
    │   │   ├── logger/                # Logging system
    │   │   │   ├── logger.go
    │   │   │   └── types.go
    │   │   ├── marketdata/            # Market data types
    │   │   │   └── types.go
    │   │   ├── models/                # Data models (math & orders)
    │   │   │   ├── math.go
    │   │   │   └── order.go
    │   │   ├── portfolio/             # P&L tracking
    │   │   │   ├── pnl_manager.go
    │   │   │   └── types.go
    │   │   ├── risk/                  # Risk management
    │   │   │   ├── risk_manager.go
    │   │   │   └── types.go
    │   │   └── tradovate/             # WebSocket & API
    │   │       ├── subscription_manager.go
    │   │       ├── websocket_manager.go
    │   │       └── types.go
    │   ├── strategies/
    │   │   └── MACrossover.go         # MA Crossover strategy
    │   ├── UI/                        # TUI implementation
    │   │   ├── UIPanel.go             
    │   │   └── types.go               
    │   └── tests/                     # Test files
    │       ├── crossover_tests.go
    │       ├── risk_tests.go
    │       ├── sma_tests.go
    │       └── testRunner.go
    ├── config/
    │   └── config.json                # Created on first run
    ├── external/
    │   └── logs/                      # Exported logs
    ├── go.mod                         # Go module definition
    └── go.sum                         # Dependency checksums
```

## Running the Application

### Normal Execution

```bash
./trading-engine
```
```

### Startup Sequence

From `cmd/main.go` lines 12-20:

```go
func main() {

	tests.RunAllTests()

	p := tea.NewProgram(UI.InitialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}

}
```

**What happens:**
1. **Tests Run**: Automated tests execute (adds ~1-2 seconds to startup)
2. **UI Initializes**: Bubbletea TUI starts in alternate screen mode
3. **Logs Display**: System log shows initialization messages
4. **Ready for Commands**: Engine awaits user input

To skip tests in production, comment out line 14 in `cmd/main.go`.

## Verification

After installation and configuration, verify the setup:

1. **Application Starts**: TUI interface displays without errors
2. **Config Loads**: No "config not found" errors in System Log
3. **Connection Works**: `Shift + 1` successfully connects to Tradovate
4. **Account Info Retrieved**: System Log shows account ID and user details

## Next Steps

Once setup is complete:
1. See [README.md](README.md) for usage instructions
2. See [ARCHITECTURE.md](ARCHITECTURE.md) for technical details
3. Configure your first strategy
4. Test with demo account before live trading

## Troubleshooting

### "Config file not found"
**Solution**: Let the application create the default config on first run, then edit it.

### "Failed to read config file"
**Solution**: Check JSON syntax is valid. Use a JSON validator if needed.

### "Failed to parse config file"
**Solution**: Ensure all required fields are present and properly typed (strings in quotes, numbers without quotes).

### "Authorization failed"
**Solution**: 
1. Verify all credentials are correct
2. Ensure `environment` matches account type
3. Try demo credentials first to test connection
4. Check if credentials have expired

### Tests failing on startup
**Solution**: Check System Log for specific test failures. May indicate code issues or incompatibilities.

## Support

For setup issues:
- Review this document thoroughly
- Check System Log for specific error messages
- Verify all prerequisites are met

---

**Ready to trade? See [README.md](README.md) for usage instructions.**