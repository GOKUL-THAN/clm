package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// cliHTTPClient enforces a strict 30-second timeout on all outbound CLI API requests.
var cliHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

// apiGet performs an authenticated GET request to the CLM API.
func apiGet(path string) (map[string]interface{}, error) {
	return doRequest("GET", path, nil)
}

// apiPost performs an authenticated POST request to the CLM API.
func apiPost(path string, body interface{}) (map[string]interface{}, error) {
	return doRequest("POST", path, body)
}

func doRequest(method, path string, body interface{}) (map[string]interface{}, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	url := apiURL + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}

	resp, err := cliHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		// Return raw body if not JSON
		return map[string]interface{}{"response": string(respBody)}, nil
	}

	return result, nil
}

// printJSON pretty-prints a map as JSON to stdout.
func printJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
