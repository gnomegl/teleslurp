package commands

import (
	"context"
	"fmt"

	"github.com/gnomegl/teleslurp/internal/config"
	"github.com/gnomegl/teleslurp/internal/telegram"
	"github.com/gnomegl/teleslurp/internal/tgscan"
	"github.com/gnomegl/teleslurp/internal/types"
	"github.com/spf13/cobra"
)

var (
	apiKey    string
	apiID     int
	apiHash   string
	noPrompt  bool
)

func init() {
	searchCmd := &cobra.Command{
		Use:   "search [username]",
		Short: "Search for a Telegram user",
		Long: `Search for a Telegram user and display their information including:
- Basic user details
- Username history
- ID history
- Group memberships`,
		Args: cobra.ExactArgs(1),
		RunE: runSearch,
	}

	searchCmd.Flags().StringVar(&apiKey, "api-key", "", "TGScan API key")
	searchCmd.Flags().IntVar(&apiID, "api-id", 0, "Telegram API ID")
	searchCmd.Flags().StringVar(&apiHash, "api-hash", "", "Telegram API Hash")
	searchCmd.Flags().BoolVar(&noPrompt, "no-prompt", false, "Disable interactive prompts")

	rootCmd.AddCommand(searchCmd)
}

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

func runSearch(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	if cfg == nil {
		cfg = &config.Config{}
	}

	// Override config with command line flags if provided
	if apiKey != "" {
		cfg.APIKey = apiKey
	}
	if apiID != 0 {
		cfg.TGAPIID = apiID
	}
	if apiHash != "" {
		cfg.TGAPIHash = apiHash
	}

	// Prompt for missing credentials if needed and not disabled
	if !noPrompt {
		if cfg.APIKey == "" {
			cfg.APIKey = promptAPIKey()
		}

		if cfg.TGAPIID == 0 || cfg.TGAPIHash == "" {
			cfg.TGAPIID, cfg.TGAPIHash = promptTGCredentials()
		}
	}

	// Validate required credentials
	if cfg.APIKey == "" || cfg.TGAPIID == 0 || cfg.TGAPIHash == "" {
		return fmt.Errorf("missing required credentials. Use flags or enable prompts")
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("error saving config: %w", err)
	}

	query := args[0]
	
	tgScanResp, err := tgscan.SearchUser(cfg.APIKey, query)
	if err != nil {
		return fmt.Errorf("error searching user: %w", err)
	}

	printUserInfo(tgScanResp)

	ctx := context.Background()
	if err := telegram.RunClient(ctx, cfg, &tgScanResp.Result.User, tgScanResp.Result.Groups); err != nil {
		return fmt.Errorf("error running Telegram client: %w", err)
	}

	return nil
}
func printUserInfo(tgScanResp *types.TGScanResponse) {
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
}
