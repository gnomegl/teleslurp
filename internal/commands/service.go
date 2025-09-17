package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/gnomegl/teleslurp/internal/config"
	"github.com/gnomegl/teleslurp/internal/database"
	"github.com/gnomegl/teleslurp/internal/telegram"
	"github.com/spf13/cobra"
)

var (
	targetChannelID int64
	channelIDsStr   string
	apiKey          string
	apiID           int
	apiHash         string
	noPrompt        bool
)

func init() {
	serviceCmd := &cobra.Command{
		Use:   "service",
		Short: "Run as a service to monitor Telegram chats",
		Long: `Run teleslurp as a service to continuously monitor specified Telegram chats and forward messages to a target channel.
Example: teleslurp service --channel-ids=123456789,987654321 --target-channel=123456`,
		RunE: runService,
	}

	serviceCmd.Flags().Int64Var(&targetChannelID, "target-channel", 0, "Target channel ID to forward messages to")
	serviceCmd.Flags().StringVar(&channelIDsStr, "channel-ids", "", "Comma-separated list of channel IDs to monitor")
	serviceCmd.Flags().StringVar(&apiKey, "api-key", "", "TGScan API key")
	serviceCmd.Flags().IntVar(&apiID, "api-id", 0, "Telegram API ID")
	serviceCmd.Flags().StringVar(&apiHash, "api-hash", "", "Telegram API Hash")
	serviceCmd.Flags().BoolVar(&noPrompt, "no-prompt", false, "Disable interactive prompts")

	serviceCmd.MarkFlagRequired("channel-ids")
	serviceCmd.MarkFlagRequired("target-channel")

	rootCmd.AddCommand(serviceCmd)
}

func runService(cmd *cobra.Command, args []string) error {
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

	dbDir := filepath.Dir("teleslurp.db")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("error creating database directory: %w", err)
	}

	db, err := database.New("teleslurp.db")
	if err != nil {
		return fmt.Errorf("error initializing database: %w", err)
	}
	defer db.Close()

	var channelIDs []int64
	for _, idStr := range strings.Split(channelIDsStr, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid channel ID %q: %w", idStr, err)
		}
		channelIDs = append(channelIDs, id)
	}

	client := telegram.NewClient(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived shutdown signal. Gracefully shutting down...")
		cancel()
	}()

	fmt.Printf("Starting teleslurp service...\nMonitoring %d channels and forwarding to channel %d\n", len(channelIDs), targetChannelID)

	return client.MonitorAndForward(ctx, channelIDs, targetChannelID)
}
