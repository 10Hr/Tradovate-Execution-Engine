package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	// Use demo API endpoint for your eval account
	baseURL        = "https://demo.tradovateapi.com/v1"
	webSocketMDURL = "wss://demo.tradovateapi.com/v1/websocket"

	symbol = "ESH6"

	//accessToken   = "eyJraWQiOiIxNyIsImFsZyI6IkVkRFNBIn0.eyJzdWIiOiI2Mzc1MzI1IiwiZXhwIjoxNzY4MzQzMjgxLCJqdGkiOiIxNjAzMjE2NDc1NjgyNzM0MTYxLS00NzQxMzg5MjMzNDEwMDAwMDcwIiwicGhzIjotMTI4MDUyMzM3N30.i8ypxsl5U9tRLTbfZMrhApbEXQSI6d0IhEoBBJAFKJbKSX4TLDdKGAs9ogtTS3l7GOs64sxp9Ly2DYeY41_wBQ"
	mdAccessToken = "eyJraWQiOiIxNyIsImFsZyI6IkVkRFNBIn0.eyJzdWIiOiI2Mzc1MzI1IiwiZXhwIjoxNzY4MzQzMjgxLCJqdGkiOiItMjA0NTg3ODM4NTY2NTk2NzkzMy01OTg2NjUyNTI1MjkzMjYzOTI3IiwicGhzIjotMTI4MDUyMzM3NywiYWNsIjoie1wiZW50cmllc1wiOntcIlVzZXJzXCI6XCJSZWFkXCJ9LFwicmVwb3J0c1wiOnt9LFwiZGVmYXVsdFwiOlwiRGVuaWVkXCJ9In0.moZJLxwlXc5ub1bNIwSWHw9rcqtoK1KcFG1d3rzkUESm3DXyZ7oNiaiEpluxo-qnxqqa_fSRfmhglJ6jmpU0Aw"

	// Your tokens
	accessToken = "eyJraWQiOiIxNyIsImFsZyI6IkVkRFNBIn0.eyJzdWIiOiI2Mzc1MzI1IiwiZXhwIjoxNzY4NTgzNTc0LCJqdGkiOiItNjYyNjYwOTM0ODk1OTU2OTk3OC0tMzQxNDA4ODY3NDE5NDMzMzkxNyIsInBocyI6LTEyODA1MjMzNzd9.8G3WsFSP4aH7FsA95pj75_MQLgOfCWkXd_kW5dAHWiNlgK_e7py98PNgYB-lYlp3jWJJ5S_UlgEchUbTRvUZBw"
	//mdAccessToken = "eyJraWQiOiIxNyIsImFsZyI6IkVkRFNBIn0.eyJzdWIiOiI2Mzc1MzI1IiwiZXhwIjoxNzY4Mjg0MTU4LCJqdGkiOiItMzk0ODk1NzE2OTk2MDQzOTc3MC0tNTcwMjM1OTQ0MDcxMjY0NjMzNSIsInBocyI6LTEyODA1MjMzNzcsImFjbCI6IntcImVudHJpZXNcIjp7XCJVc2Vyc1wiOlwiUmVhZFwifSxcInJlcG9ydHNcIjp7fSxcImRlZmF1bHRcIjpcIkRlbmllZFwifSJ9.-HH-DoxUIAXLYjA8FLPC_ieeO2ChdfMEWugC8yQh_EG3BAoMiVxBwK5PenoMMewPHuBBxfKbKiUQyXgjLmCyCA"
)

func makeAuthenticatedRequest(endpoint string) ([]byte, error) {
	url := baseURL + endpoint

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add authorization header
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func makeMarketDataRequest(endpoint string) ([]byte, error) {
	url := baseURL + endpoint

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Use mdAccessToken for market data endpoints
	req.Header.Set("Authorization", "Bearer "+mdAccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func main() {
	// Test 1: Get account list
	fmt.Println("Testing connection to Tradovate API...")

	data, err := makeAuthenticatedRequest("/account/list")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Success! Account data:")

	// Pretty print the JSON
	var result interface{}
	json.Unmarshal(data, &result)
	prettyJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(prettyJSON))

	fmt.Println("\nTesting market data...")

	// Search for contracts by name
	data2, err2 := makeMarketDataRequest("/contract/find?name=" + symbol)
	if err2 != nil {
		fmt.Printf("Error: %v\n", err2)
		return
	}

	fmt.Println("Contract data:")
	var result2 interface{}
	json.Unmarshal(data2, &result2)
	prettyJSON2, _ := json.MarshalIndent(result2, "", "  ")
	fmt.Println(string(prettyJSON2))

}
