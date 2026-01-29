package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"tradovate-execution-engine/engine/config"
	"tradovate-execution-engine/engine/internal/logger"
	"tradovate-execution-engine/engine/internal/tradovate"
)

var (
	// Global token manager instance
	globalTokenManager *TokenManager
	once               sync.Once
)

// NewTokenManager returns the singleton token manager instance
func NewTokenManager(config *config.Config) *TokenManager {
	once.Do(func() {
		globalTokenManager = &TokenManager{}
	})

	globalTokenManager.SetCredentials(
		config.Tradovate.AppID,
		config.Tradovate.AppVersion,
		config.Tradovate.Chl,
		config.Tradovate.Cid,
		config.Tradovate.DeviceID,
		config.Tradovate.Environment,
		config.Tradovate.Username,
		config.Tradovate.Password,
		config.Tradovate.Sec,
		config.Tradovate.Enc,
	)

	return globalTokenManager
}

// ResetTokenManagerForTest resets the singleton for testing purposes
func ResetTokenManagerForTest() {
	globalTokenManager = nil
	once = sync.Once{}
}

// SetLogger sets the logger for the TokenManager
func (tm *TokenManager) SetLogger(l *logger.Logger) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.log = l
}

// SetCredentials stores the authentication credentials
func (tm *TokenManager) SetCredentials(appID, appVersion, chl, cid, deviceID, environment, name, password, sec string, enc bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.credentials = map[string]interface{}{
		"appId":       appID,
		"appVersion":  appVersion,
		"chl":         chl,
		"cid":         cid,
		"deviceId":    deviceID,
		"enc":         enc,
		"environment": environment,
		"name":        name,
		"password":    password,
		"sec":         sec,
	}

	tm.baseURL = config.GetHTTPBaseURL(environment)
}

// Authenticate performs authentication and stores tokens
func (tm *TokenManager) Authenticate() error {
	tm.mu.RLock()
	credentials := tm.credentials
	baseURL := tm.baseURL
	tm.mu.RUnlock()

	if credentials == nil {
		return fmt.Errorf("Credentials not set. Call SetCredentials first")
	}

	// Marshal credentials to JSON
	jsonData, err := json.Marshal(credentials)
	if err != nil {
		return fmt.Errorf("Error marshaling credentials: %w", err)
	}

	// Create the request
	req, err := http.NewRequest("POST", baseURL+"/v1/auth/accesstokenrequest", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("Error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Make the request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Error making request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)

	if err != nil {
		return fmt.Errorf("Error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return parseAuthError(resp.StatusCode, respBody)
	}

	// Parse response
	var authResp tradovate.APIAuthResponse
	if err := json.Unmarshal(respBody, &authResp); err != nil {
		return fmt.Errorf("Error parsing response: %w", err)
	}

	// Store tokens
	tm.mu.Lock()
	tm.accessToken = authResp.AccessToken
	tm.mdAccessToken = authResp.MDAccessToken
	tm.expirationTime = authResp.ExpirationTime
	tm.userID = authResp.UserID
	tm.username = authResp.Name
	tm.mu.Unlock()

	return nil
}

// parseAuthError parses HTTP errors from Tradovate
func parseAuthError(statusCode int, body []byte) error {
	switch statusCode {
	case 400:
		return fmt.Errorf("invalid credentials format (check username/password)")
	case 401:
		return fmt.Errorf("authentication failed - incorrect username or password")
	case 403:
		return fmt.Errorf("access denied - check App ID, CID, and SEC")
	case 429:
		return fmt.Errorf("too many login attempts - please wait and try again")
	case 500, 502, 503:
		return fmt.Errorf("Tradovate API error - please try again later")
	default:
		// Try to parse error message from response
		var apiErr struct {
			ErrorText string `json:"errorText"`
			Message   string `json:"message"`
		}
		if err := json.Unmarshal(body, &apiErr); err == nil {
			if apiErr.ErrorText != "" {
				return fmt.Errorf("authentication failed: %s", apiErr.ErrorText)
			}
			if apiErr.Message != "" {
				return fmt.Errorf("authentication failed: %s", apiErr.Message)
			}
		}
		return fmt.Errorf("authentication failed with status %d: %s", statusCode, string(body))
	}
}

// GetAccessToken returns the current access token
func (tm *TokenManager) GetAccessToken() (string, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.accessToken == "" {
		return "", fmt.Errorf("No access token available. Call Authenticate first")
	}

	if time.Now().After(tm.expirationTime) {
		return "", fmt.Errorf("Access token expired. Please re-authenticate")
	}

	return tm.accessToken, nil
}

// GetMDAccessToken returns the market data access token
func (tm *TokenManager) GetMDAccessToken() (string, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.mdAccessToken == "" {
		return "", fmt.Errorf("No MD access token available. Call Authenticate first")
	}

	if time.Now().After(tm.expirationTime) {
		return "", fmt.Errorf("MD access token expired. Please re-authenticate")
	}

	return tm.mdAccessToken, nil
}

// GetUserID returns the authenticated user ID
func (tm *TokenManager) GetUserID() int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.userID
}

// GetUsername returns the authenticated username
func (tm *TokenManager) GetUsername() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.username
}

// GetAccountID returns the cached account ID, fetching it if necessary
func (tm *TokenManager) GetAccountID() (int, error) {
	tm.mu.RLock()
	if tm.accountID != 0 {
		defer tm.mu.RUnlock()
		return tm.accountID, nil
	}
	tm.mu.RUnlock()

	// Ensure we have a token
	token, err := tm.GetAccessToken()
	if err != nil {
		return 0, err
	}

	// Fetch accounts
	resp, err := tm.MakeAuthenticatedRequest("GET", "/v1/account/list", nil, token)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("failed to list accounts: %s", string(body))
	}

	var accounts []tradovate.APIAccount
	if err := json.NewDecoder(resp.Body).Decode(&accounts); err != nil {
		return 0, fmt.Errorf("failed to decode account list: %w", err)
	}

	if len(accounts) == 0 {
		return 0, fmt.Errorf("no accounts found")
	}

	// Cache the first account ID
	tm.mu.Lock()
	tm.accountID = accounts[0].ID
	tm.mu.Unlock()

	if tm.log != nil {
		tm.log.Debugf("Using Account ID: %d (%s)", accounts[0].ID, accounts[0].Name)
	}

	return accounts[0].ID, nil
}

// IsAuthenticated checks if there's a valid access token
func (tm *TokenManager) IsAuthenticated() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.accessToken != "" && time.Now().Before(tm.expirationTime)
}

// GetBaseURL returns the base API URL
func (tm *TokenManager) GetBaseURL() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.baseURL
}

// MakeAuthenticatedRequest makes an HTTP request with authentication
func (tm *TokenManager) MakeAuthenticatedRequest(method, endpoint string, body interface{}, token string) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("Error marshaling request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	baseURL := tm.GetBaseURL()
	req, err := http.NewRequest(method, baseURL+endpoint, reqBody)
	if err != nil {
		return nil, fmt.Errorf("Error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

// RenewAccessToken renews the current access token without creating a new session
// This should be used instead of Authenticate() for long-running applications
func (tm *TokenManager) RenewAccessToken() error {
	tm.mu.RLock()
	currentToken := tm.accessToken
	baseURL := tm.baseURL
	tm.mu.RUnlock()

	if currentToken == "" {
		return fmt.Errorf("No current token available. Call Authenticate first")
	}

	// Create the request
	req, err := http.NewRequest("GET", baseURL+"/v1/auth/renewaccesstoken", nil)
	if err != nil {
		return fmt.Errorf("error creating renewal request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+currentToken)

	// Make the request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Error making renewal request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Error reading renewal response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Token renewal failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response (same structure as auth response)
	var renewResp tradovate.APIAuthResponse
	if err := json.Unmarshal(respBody, &renewResp); err != nil {
		return fmt.Errorf("Error parsing renewal response: %w", err)
	}

	// Update tokens in memory (WebSockets stay connected!)
	tm.mu.Lock()
	tm.accessToken = renewResp.AccessToken
	tm.expirationTime = renewResp.ExpirationTime
	tm.mu.Unlock()

	// Log success
	tm.mu.RLock()
	if tm.log != nil {
		tm.log.Debug("Token renewed successfully")
	}
	tm.mu.RUnlock()

	return nil
}

// StartTokenRefreshMonitor starts a background goroutine that refreshes tokens before expiration
func (tm *TokenManager) StartTokenRefreshMonitor(refreshCallback func()) {
	tm.mu.Lock()
	if tm.monitorStopChan != nil {
		close(tm.monitorStopChan)
	}
	tm.monitorStopChan = make(chan struct{})
	stopChan := tm.monitorStopChan
	tm.mu.Unlock()

	go func() {
		for {
			// Calculate time until token expires
			tm.mu.RLock()
			timeUntilExpiry := time.Until(tm.expirationTime)
			tm.mu.RUnlock()

			// Refresh 5 minutes before expiration (safer than 10 min before)
			refreshTime := timeUntilExpiry - (5 * time.Minute)

			// If already expired or expiring soon, refresh immediately
			if refreshTime <= 0 {
				refreshTime = 1 * time.Second
			}

			tm.log.Debugf("Token refresh scheduled in %v (expires at %v)",
				refreshTime, tm.expirationTime.Format("3:04 PM"))

			// Wait until refresh time or stop signal
			select {
			case <-stopChan:
				tm.log.Info("Token refresh monitor stopped")
				return
			case <-time.After(refreshTime):
				// Continue to refresh
			}

			if tm.log != nil {
				tm.log.Debug("Refreshing access tokens...")
			}

			if err := tm.RenewAccessToken(); err != nil {
				tm.log.Errorf("Failed to renew access token: %v", err)
			} else {
				if tm.log != nil {
					tm.log.Debug("Tokens refreshed successfully")
				}

				// Call the callback to notify that tokens have been refreshed
				if refreshCallback != nil {
					refreshCallback()
				}
			}
		}
	}()
}

// StopTokenRefreshMonitor stops the background refresh goroutine
func (tm *TokenManager) StopTokenRefreshMonitor() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.monitorStopChan != nil {
		close(tm.monitorStopChan)
		tm.monitorStopChan = nil
	}
}
