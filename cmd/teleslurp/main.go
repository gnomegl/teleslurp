package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gnomegl/teleslurp/internal/config"
	"github.com/gnomegl/teleslurp/internal/telegram"
	"github.com/gnomegl/teleslurp/internal/tgscan"
)

func promptAPIKey() string {
	fmt.Print("Please enter your TGScan API key: ")
	var apiKey string
	fmt.Scanln(&apiKey)
	return apiKey
}

func promptTGCredentials() (int, string) {
	var apiID int
	var apiHash string

	fmt.Print("Please enter your Telegram API ID: ")
	fmt.Scanln(&apiID)

	fmt.Print("Please enter your Telegram API Hash: ")
	fmt.Scanln(&apiHash)

	return apiID, apiHash
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: teleslurp <search_query>")
		fmt.Println("Example: teleslurp ytcracka")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if cfg == nil {
		cfg = &config.Config{}
	}

	if cfg.APIKey == "" {
		cfg.APIKey = promptAPIKey()
	}

	if cfg.TGAPIID == 0 || cfg.TGAPIHash == "" {
		cfg.TGAPIID, cfg.TGAPIHash = promptTGCredentials()
	}

	if err := config.Save(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	query := os.Args[1]
	
	tgScanResp, err := tgscan.SearchUser(cfg.APIKey, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error searching user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User Information:\n")
	fmt.Printf("ID: %d\n", tgScanResp.Result.User.ID)
	fmt.Printf("Username: %s\n", tgScanResp.Result.User.Username)
	fmt.Printf("First Name: %s\n", tgScanResp.Result.User.FirstName)
	fmt.Printf("Last Name: %s\n", tgScanResp.Result.User.LastName)

	fmt.Println("\nUsername History:")
	for _, history := range tgScanResp.Result.UsernameHistory {
		fmt.Printf("  - Username: %s (Changed on: %s)\n", history.Username, history.Date)
	}

	fmt.Println("\nID History:")
	for _, history := range tgScanResp.Result.IDHistory {
		fmt.Printf("  - ID: %d (Changed on: %s)\n", history.ID, history.Date)
	}

	fmt.Println("\nMeta Information:")
	fmt.Printf("Search Query: %s\n", tgScanResp.Result.Meta.SearchQuery)
	fmt.Printf("Known Number of Groups: %d\n", tgScanResp.Result.Meta.KnownNumGroups)
	fmt.Printf("Number of Groups: %d\n", tgScanResp.Result.Meta.NumGroups)
	fmt.Printf("Operation Cost: %d\n", tgScanResp.Result.Meta.OpCost)

	fmt.Println("\nGroups:")
	for _, group := range tgScanResp.Result.Groups {
		fmt.Printf("  - Group Name: %s\n", group.Title)
		fmt.Printf("    Username: %s\n", group.Username)
		fmt.Printf("    Date Updated: %s\n", group.DateUpdated)
	}
	fmt.Println("")

	ctx := context.Background()
	if err := telegram.RunClient(ctx, cfg, &tgScanResp.Result.User, tgScanResp.Result.Groups); err != nil {
		fmt.Fprintf(os.Stderr, "Error running Telegram client: %v\n", err)
		os.Exit(1)
	}
}
