package tgscan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gnomegl/teleslurp/internal/types"
)

func SearchUser(apiKey, query string) (*types.TGScanResponse, error) {
	data := []byte(fmt.Sprintf("query=%s", query))

	httpClient := &http.Client{}
	req, err := http.NewRequest("POST", "https://api.tgdev.io/tgscan/v1/search", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Api-Key", apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	var tgScanResp types.TGScanResponse
	if err := json.Unmarshal(body, &tgScanResp); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	// Check if user was actually found
	if tgScanResp.Result.User.ID == 0 && tgScanResp.Result.User.Username == "" {
		return &tgScanResp, fmt.Errorf("user not found")
	}

	return &tgScanResp, nil
}
