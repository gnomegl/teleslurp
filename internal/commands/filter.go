package commands

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gnomegl/teleslurp/internal/config"
	"github.com/gnomegl/teleslurp/internal/database"
	"github.com/gnomegl/teleslurp/internal/filter"
	"github.com/spf13/cobra"
)

var (
	filterName     string
	filterType     string
	filterPattern  string
	filterAction   string
	filterPriority int
	filterID       int
)

func init() {
	filterCmd := &cobra.Command{
		Use:   "filter",
		Short: "Manage message filters",
		Long:  `Manage message filters for the monitoring system`,
	}

	// Add filter subcommand
	addFilterCmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new message filter",
		Long: `Add a new message filter to control which messages are forwarded.

Filter types:
- keyword: Filter messages containing specific keywords
- regex: Filter messages matching a regex pattern
- user: Filter messages from specific user IDs
- channel: Filter messages from specific channel IDs
- length: Filter messages based on minimum length

Actions:
- forward: Forward the message (default)
- ignore: Do not forward the message
- highlight: Forward with special highlighting`,
		RunE: runAddFilter,
	}

	addFilterCmd.Flags().StringVarP(&filterName, "name", "n", "", "Filter name (required)")
	addFilterCmd.Flags().StringVarP(&filterType, "type", "t", "", "Filter type: keyword, regex, user, channel, length (required)")
	addFilterCmd.Flags().StringVarP(&filterPattern, "pattern", "p", "", "Filter pattern (required)")
	addFilterCmd.Flags().StringVarP(&filterAction, "action", "a", "forward", "Action: forward, ignore, highlight")
	addFilterCmd.Flags().IntVarP(&filterPriority, "priority", "P", 0, "Filter priority (higher = evaluated first)")
	addFilterCmd.MarkFlagRequired("name")
	addFilterCmd.MarkFlagRequired("type")
	addFilterCmd.MarkFlagRequired("pattern")

	// List filters subcommand
	listFiltersCmd := &cobra.Command{
		Use:   "list",
		Short: "List all message filters",
		RunE:  runListFilters,
	}

	// Enable filter subcommand
	enableFilterCmd := &cobra.Command{
		Use:   "enable [filter-id]",
		Short: "Enable a message filter",
		Args:  cobra.ExactArgs(1),
		RunE:  runEnableFilter,
	}

	// Disable filter subcommand
	disableFilterCmd := &cobra.Command{
		Use:   "disable [filter-id]",
		Short: "Disable a message filter",
		Args:  cobra.ExactArgs(1),
		RunE:  runDisableFilter,
	}

	filterCmd.AddCommand(addFilterCmd, listFiltersCmd, enableFilterCmd, disableFilterCmd)
	rootCmd.AddCommand(filterCmd)
}

func runAddFilter(cmd *cobra.Command, args []string) error {
	// Initialize database
	dbPath := config.GetDatabasePath()
	db, err := database.New(dbPath)
	if err != nil {
		return fmt.Errorf("error initializing database: %w", err)
	}
	defer db.Close()

	// Validate filter type
	validTypes := map[string]bool{
		"keyword": true,
		"regex":   true,
		"user":    true,
		"channel": true,
		"length":  true,
	}
	if !validTypes[filterType] {
		return fmt.Errorf("invalid filter type: %s", filterType)
	}

	// Validate action
	validActions := map[string]bool{
		"forward":   true,
		"ignore":    true,
		"highlight": true,
	}
	if !validActions[filterAction] {
		return fmt.Errorf("invalid action: %s", filterAction)
	}

	// Add filter based on type
	switch filterType {
	case "keyword":
		keywords := strings.Split(filterPattern, ",")
		err = filter.AddKeywordFilter(db, filterName, keywords, filterAction, filterPriority)
	case "regex":
		err = filter.AddRegexFilter(db, filterName, filterPattern, filterAction, filterPriority)
	case "user":
		userIDs := parseInt64List(filterPattern)
		err = filter.AddUserFilter(db, filterName, userIDs, filterAction, filterPriority)
	case "channel":
		channelIDs := parseInt64List(filterPattern)
		err = filter.AddChannelFilter(db, filterName, channelIDs, filterAction, filterPriority)
	case "length":
		minLength, parseErr := strconv.Atoi(filterPattern)
		if parseErr != nil {
			return fmt.Errorf("invalid length value: %s", filterPattern)
		}
		err = filter.AddLengthFilter(db, filterName, minLength, filterAction, filterPriority)
	}

	if err != nil {
		return fmt.Errorf("error adding filter: %w", err)
	}

	fmt.Printf("Filter '%s' added successfully\n", filterName)
	return nil
}

func runListFilters(cmd *cobra.Command, args []string) error {
	// Initialize database
	dbPath := config.GetDatabasePath()
	db, err := database.New(dbPath)
	if err != nil {
		return fmt.Errorf("error initializing database: %w", err)
	}
	defer db.Close()

	// Get all filters
	filters, err := db.GetActiveFilters()
	if err != nil {
		return fmt.Errorf("error getting filters: %w", err)
	}

	if len(filters) == 0 {
		fmt.Println("No filters configured")
		return nil
	}

	fmt.Println("Active Message Filters:")
	fmt.Println("========================")
	for _, f := range filters {
		status := "enabled"
		fmt.Printf("ID: %d | Name: %s | Type: %s | Pattern: %s | Action: %s | Priority: %d | Status: %s\n",
			f.ID, f.Name, f.Type, f.Pattern, f.Action, f.Priority, status)
	}

	return nil
}

func runEnableFilter(cmd *cobra.Command, args []string) error {
	filterID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid filter ID: %s", args[0])
	}

	// Initialize database
	dbPath := config.GetDatabasePath()
	db, err := database.New(dbPath)
	if err != nil {
		return fmt.Errorf("error initializing database: %w", err)
	}
	defer db.Close()

	if err := db.EnableFilter(filterID); err != nil {
		return fmt.Errorf("error enabling filter: %w", err)
	}

	fmt.Printf("Filter %d enabled\n", filterID)
	return nil
}

func runDisableFilter(cmd *cobra.Command, args []string) error {
	filterID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid filter ID: %s", args[0])
	}

	// Initialize database
	dbPath := config.GetDatabasePath()
	db, err := database.New(dbPath)
	if err != nil {
		return fmt.Errorf("error initializing database: %w", err)
	}
	defer db.Close()

	if err := db.DisableFilter(filterID); err != nil {
		return fmt.Errorf("error disabling filter: %w", err)
	}

	fmt.Printf("Filter %d disabled\n", filterID)
	return nil
}

func parseInt64List(pattern string) []int64 {
	parts := strings.Split(pattern, ",")
	var ids []int64
	for _, part := range parts {
		if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
