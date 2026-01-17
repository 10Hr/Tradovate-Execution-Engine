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
)

// AuthResponse represents the Tradovate authentication response
type AuthResponse struct {
	AccessToken    string    `json:"accessToken"`
	ExpirationTime time.Time `json:"expirationTime"`
	MDAccessToken  string    `json:"mdAccessToken"`
	UserID         int       `json:"userId"`
	Name           string    `json:"name"`
	OrgName        string    `json:"orgName"`
	UserStatus     string    `json:"userStatus"`
	HasMarketData  bool      `json:"hasMarketData"`
	HasFunded      bool      `json:"hasFunded"`
	HasLive        bool      `json:"hasLive"`
}

// Account represents a Tradovate account
type Account struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	AccountType string `json:"accountType"`
	Active      bool   `json:"active"`
}

// TokenManager manages authentication tokens for Tradovate API
type TokenManager struct {
	mu             sync.RWMutex
	accessToken    string
	mdAccessToken  string
	expirationTime time.Time
	userID         int
	accountID      int
	username       string
	credentials    map[string]interface{}
	baseURL        string
	log            *logger.Logger
}

var (
	// Global token manager instance
	globalTokenManager *TokenManager
	once               sync.Once
)

// GetTokenManager returns the singleton token manager instance
func GetTokenManager() *TokenManager {
	once.Do(func() {
		globalTokenManager = &TokenManager{
			baseURL: "https://live.tradovateapi.com",
		}
	})
	return globalTokenManager
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

	// Set base URL based on environment
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
		return fmt.Errorf("Authentication failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var authResp AuthResponse
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

	// Log success if logger is set
	tm.mu.RLock()
	if tm.log != nil {
		tm.log.Info("Authentication successful!")
		tm.log.Infof("User: %s (ID: %d)", authResp.Name, authResp.UserID)
		tm.log.Infof("Org: %s", authResp.OrgName)
		tm.log.Infof("Token expires: %s", authResp.ExpirationTime.Format("January 2, 2006 at 3:04 PM EST"))
	}
	tm.mu.RUnlock()

	return nil
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

	var accounts []Account
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
		tm.log.Infof("Using Account ID: %d (%s)", accounts[0].ID, accounts[0].Name)
	}

	return accounts[0].ID, nil
}

// IsAuthenticated checks if there's a valid token
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
