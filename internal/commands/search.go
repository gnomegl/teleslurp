package commands

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"

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
	exportJSON bool
	exportCSV  bool
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
	searchCmd.Flags().BoolVar(&exportJSON, "json", false, "Export results to JSON file")
	searchCmd.Flags().BoolVar(&exportCSV, "csv", false, "Export results to CSV file")

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

	if apiKey != "" {
		cfg.APIKey = apiKey
	}
	if apiID != 0 {
		cfg.TGAPIID = apiID
	}
	if apiHash != "" {
		cfg.TGAPIHash = apiHash
	}

	if !noPrompt {
		if cfg.APIKey == "" {
			cfg.APIKey = promptAPIKey()
		}

		if cfg.TGAPIID == 0 || cfg.TGAPIHash == "" {
			cfg.TGAPIID, cfg.TGAPIHash = promptTGCredentials()
		}
	}

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

	if exportJSON {
		if err := exportToJSON(tgScanResp, query); err != nil {
			return fmt.Errorf("error exporting to JSON: %w", err)
		}
	} else if exportCSV {
		if err := exportToCSV(tgScanResp, query); err != nil {
			return fmt.Errorf("error exporting to CSV: %w", err)
		}
	} else {
		printUserInfo(tgScanResp)
	}

	var format telegram.OutputFormat
	if exportJSON {
		format = telegram.FormatJSON
	} else if exportCSV {
		format = telegram.FormatCSV
	} else {
		format = telegram.FormatJSON
	}

	ctx := context.Background()
	if err := telegram.RunClient(ctx, cfg, &types.User{Username: query}, tgScanResp.Result.Groups, format); err != nil {
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

func exportToJSON(resp *types.TGScanResponse, username string) error {
	filename := username + "_tgscan.json"
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("error creating JSON file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(resp); err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}

	fmt.Printf("Results exported to JSON file: %s\n", filename)
	return nil
}

func exportToCSV(resp *types.TGScanResponse, username string) error {
	// Export username history
	if err := exportUsernameHistoryCSV(resp); err != nil {
		return err
	}

	// Export groups
	if err := exportGroupsCSV(resp, username); err != nil {
		return err
	}

	return nil
}

func exportUsernameHistoryCSV(resp *types.TGScanResponse) error {
	historyFilename := resp.Result.User.Username + "_usernames_tgscan.csv"
	file, err := os.Create(historyFilename)
	if err != nil {
		return fmt.Errorf("error creating username history CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	headers := []string{"User ID", "Current Username", "Previous Username", "Date Changed"}
	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("error writing CSV headers: %w", err)
	}

	// Always write the current username as a record
	currentRecord := []string{
		fmt.Sprintf("%d", resp.Result.User.ID),
		resp.Result.User.Username,
		"",  // No previous username for current
		"",  // No date for current
	}
	if err := writer.Write(currentRecord); err != nil {
		return fmt.Errorf("error writing CSV record: %w", err)
	}

	// Write historical usernames if any exist
	for _, h := range resp.Result.UsernameHistory {
		record := []string{
			fmt.Sprintf("%d", resp.Result.User.ID),
			resp.Result.User.Username,
			h.Username,
			h.Date,
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("error writing CSV record: %w", err)
		}
	}

	fmt.Printf("Username history exported to CSV file: %s\n", historyFilename)
	return nil
}

func exportGroupsCSV(resp *types.TGScanResponse, username string) error {
	groupsFilename := username + "_groups_tgscan.csv"
	file, err := os.Create(groupsFilename)
	if err != nil {
		return fmt.Errorf("error creating groups CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	headers := []string{"User ID", "User Username", "Group Title", "Group Username", "Date Updated"}
	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("error writing CSV headers: %w", err)
	}

	for _, group := range resp.Result.Groups {
		record := []string{
			fmt.Sprintf("%d", resp.Result.User.ID),
			resp.Result.User.Username,
			group.Title,
			group.Username,
			group.DateUpdated,
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("error writing CSV record: %w", err)
		}
	}

	fmt.Printf("Groups exported to CSV file: %s\n", groupsFilename)
	return nil
}
