package commands

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/gnomegl/teleslurp/internal/config"
	"github.com/gnomegl/teleslurp/internal/export"
	"github.com/gnomegl/teleslurp/internal/telegram"
	"github.com/gnomegl/teleslurp/internal/tgscan"
	"github.com/gnomegl/teleslurp/internal/types"
	"github.com/spf13/cobra"
)

var (
	apiKey                string
	apiID                 int
	apiHash               string
	noPrompt              bool
	exportJSON            bool
	exportCSV             bool
	exportChannelMetadata bool
	inputFile             string
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
	searchCmd.Flags().BoolVar(&exportChannelMetadata, "metadata", false, "Export channel metadata")
	searchCmd.Flags().StringVar(&inputFile, "input-file", "", "Input file containing Telegram channels/groups to search")

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
		if cfg.TGAPIID == 0 || cfg.TGAPIHash == "" {
			cfg.TGAPIID, cfg.TGAPIHash = promptTGCredentials()
		}
	}

	if cfg.TGAPIID == 0 || cfg.TGAPIHash == "" {
		return fmt.Errorf("missing required Telegram credentials. Use flags or enable prompts")
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("error saving config: %w", err)
	}

	query := args[0]
	var searchUser types.User
	if id, err := strconv.ParseInt(query, 10, 64); err == nil {
		searchUser = types.User{ID: id}
	} else {
		searchUser = types.User{Username: query}
	}

	var groups []types.Group
	if inputFile != "" {
		channels, err := readChannelsFromFile(inputFile)
		if err != nil {
			return fmt.Errorf("error reading channels from file: %w", err)
		}
		groups = channels
	} else {
		if cfg.APIKey == "" {
			if !noPrompt {
				cfg.APIKey = promptAPIKey()
			}
			if cfg.APIKey == "" {
				return fmt.Errorf("TGScan API key is required when not using input file")
			}
		}

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

		groups = tgScanResp.Result.Groups
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
	if err := telegram.RunClient(ctx, cfg, &searchUser, groups, format, exportChannelMetadata); err != nil {
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
	filename := export.FormatFilename(username, "tgscan", "json")
	return export.WriteJSON(resp, filename)
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
	historyFilename := export.FormatFilename(resp.Result.User.Username, "usernames_tgscan", "csv")
	writer, err := export.NewCSVWriter(historyFilename)
	if err != nil {
		return err
	}
	defer writer.Close()

	headers := []string{"User ID", "Current Username", "Previous Username", "Date Changed"}
	if err := writer.WriteHeader(headers); err != nil {
		return err
	}

	// Always write the current username as a record
	currentRecord := []string{
		fmt.Sprintf("%d", resp.Result.User.ID),
		resp.Result.User.Username,
		"", // No previous username for current
		"", // No date for current
	}
	if err := writer.WriteRecord(currentRecord); err != nil {
		return err
	}

	// Write historical usernames if any exist
	for _, h := range resp.Result.UsernameHistory {
		record := []string{
			fmt.Sprintf("%d", resp.Result.User.ID),
			resp.Result.User.Username,
			h.Username,
			h.Date,
		}
		if err := writer.WriteRecord(record); err != nil {
			return err
		}
	}

	fmt.Printf("Username history exported to CSV file: %s\n", historyFilename)
	return nil
}

func exportGroupsCSV(resp *types.TGScanResponse, username string) error {
	groupsFilename := export.FormatFilename(username, "groups_tgscan", "csv")
	writer, err := export.NewCSVWriter(groupsFilename)
	if err != nil {
		return err
	}
	defer writer.Close()

	headers := []string{"User ID", "User Username", "Group Title", "Group Username", "Date Updated"}
	if err := writer.WriteHeader(headers); err != nil {
		return err
	}

	for _, group := range resp.Result.Groups {
		record := []string{
			fmt.Sprintf("%d", resp.Result.User.ID),
			resp.Result.User.Username,
			group.Title,
			group.Username,
			group.DateUpdated,
		}
		if err := writer.WriteRecord(record); err != nil {
			return err
		}
	}

	fmt.Printf("Groups exported to CSV file: %s\n", groupsFilename)
	return nil
}

func readChannelsFromFile(filename string) ([]types.Group, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	// if the file contains any t.me links, extract those
	channelRegex := regexp.MustCompile(`(?:https?://)?t\.me/(?:[a-z]/)?([0-9]+|[a-zA-Z0-9_]+)`)
	if matches := channelRegex.FindAllStringSubmatch(string(content), -1); len(matches) > 0 {
		channels := make([]types.Group, 0, len(matches))
		for _, match := range matches {
			if id, err := strconv.ParseInt(match[1], 10, 64); err == nil {
				channels = append(channels, types.Group{ID: id})
			} else {
				channels = append(channels, types.Group{Username: match[1]})
			}
		}
		return channels, nil
	}

	// otherwise treat each non empty, non comment line as a channel id or username
	var channels []types.Group
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if id, err := strconv.ParseInt(line, 10, 64); err == nil {
			channels = append(channels, types.Group{ID: id})
		} else {
			channels = append(channels, types.Group{Username: line})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning file: %w", err)
	}

	if len(channels) == 0 {
		return nil, fmt.Errorf("no valid channels found in file")
	}

	return channels, nil
}
