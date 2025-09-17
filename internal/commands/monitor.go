package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gnomegl/teleslurp/internal/config"
	"github.com/gnomegl/teleslurp/internal/database"
	"github.com/gnomegl/teleslurp/internal/telegram"
	"github.com/spf13/cobra"
)

var (
	configFile string
)

func init() {
	var (
		apiKey   string
		apiID    int
		apiHash  string
		noPrompt bool
	)

	monitorCmd := &cobra.Command{
		Use:   "monitor",
		Short: "Monitor Telegram chats and forward messages",
		Long: `Monitor specified Telegram chats and forward messages to target channels.
Example: teleslurp monitor --config=monitor.config.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMonitor(cmd, args, apiKey, apiID, apiHash, noPrompt)
		},
	}

	monitorCmd.Flags().StringVar(&configFile, "config", "", "Path to monitor configuration file")
	monitorCmd.Flags().StringVar(&apiKey, "api-key", "", "TGScan API key")
	monitorCmd.Flags().IntVar(&apiID, "api-id", 0, "Telegram API ID")
	monitorCmd.Flags().StringVar(&apiHash, "api-hash", "", "Telegram API Hash")
	monitorCmd.Flags().BoolVar(&noPrompt, "no-prompt", false, "Disable interactive prompts")

	rootCmd.AddCommand(monitorCmd)
}

// resolveSources resolves usernames to IDs for channels and groups
func resolveSources(ctx context.Context, client *telegram.Client, channels, groups []config.MonitorSource) ([]int64, error) {
	var ids []int64

	// Create a temporary context wrapper to run the client for resolution
	err := client.RunWithContext(ctx, func(ctx context.Context) error {
		// Resolve channels
		for _, ch := range channels {
			if ch.ID != 0 {
				ids = append(ids, ch.ID)
				fmt.Printf("Added channel ID: %d\n", ch.ID)
			} else if ch.Username != "" {
				channelID, _, title, err := client.ResolveChannelUsername(ctx, ch.Username)
				if err != nil {
					fmt.Printf("Warning: Could not resolve channel %s: %v\n", ch.Username, err)
					continue
				}
				ids = append(ids, channelID)
				fmt.Printf("Resolved channel @%s (%s) to ID: %d\n", ch.Username, title, channelID)
			}
		}

		// Resolve groups
		for _, grp := range groups {
			if grp.ID != 0 {
				ids = append(ids, grp.ID)
				fmt.Printf("Added group ID: %d\n", grp.ID)
			} else if grp.Username != "" {
				groupID, _, title, err := client.ResolveChannelUsername(ctx, grp.Username)
				if err != nil {
					fmt.Printf("Warning: Could not resolve group %s: %v\n", grp.Username, err)
					continue
				}
				ids = append(ids, groupID)
				fmt.Printf("Resolved group @%s (%s) to ID: %d\n", grp.Username, title, groupID)
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error resolving sources: %w", err)
	}

	return ids, nil
}

// resolveTargets resolves usernames to IDs for target channels
func resolveTargets(ctx context.Context, client *telegram.Client, targets []config.MonitorTarget) ([]int64, error) {
	var ids []int64

	// Create a temporary context wrapper to run the client for resolution
	err := client.RunWithContext(ctx, func(ctx context.Context) error {
		for _, target := range targets {
			if target.ID != 0 {
				ids = append(ids, target.ID)
				fmt.Printf("Added target channel ID: %d\n", target.ID)
			} else if target.Username != "" {
				channelID, _, title, err := client.ResolveChannelUsername(ctx, target.Username)
				if err != nil {
					fmt.Printf("Warning: Could not resolve target channel %s: %v\n", target.Username, err)
					continue
				}
				ids = append(ids, channelID)
				fmt.Printf("Resolved target channel @%s (%s) to ID: %d\n", target.Username, title, channelID)
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error resolving targets: %w", err)
	}

	return ids, nil
}

// resolveUsers resolves usernames to IDs for user monitoring
func resolveUsers(ctx context.Context, client *telegram.Client, users []config.MonitorSource) ([]int64, error) {
	var ids []int64

	// Create a temporary context wrapper to run the client for resolution
	err := client.RunWithContext(ctx, func(ctx context.Context) error {
		for _, user := range users {
			if user.ID != 0 {
				ids = append(ids, user.ID)
				fmt.Printf("Added user ID for monitoring: %d\n", user.ID)
			} else if user.Username != "" {
				userID, _, username, fullName, err := client.ResolveUserUsername(ctx, user.Username)
				if err != nil {
					fmt.Printf("Warning: Could not resolve user %s: %v\n", user.Username, err)
					continue
				}
				ids = append(ids, userID)
				fmt.Printf("Resolved user @%s (%s) to ID: %d\n", username, fullName, userID)
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error resolving users: %w", err)
	}

	return ids, nil
}

func runMonitor(cmd *cobra.Command, args []string, apiKey string, apiID int, apiHash string, noPrompt bool) error {
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

	// Load monitor configuration
	var monitorCfg *config.MonitorConfig
	if configFile != "" {
		monitorCfg, err = config.LoadMonitorConfig()
		if err != nil {
			return fmt.Errorf("error loading monitor config: %w", err)
		}
	} else {
		// Try to load default monitor config
		monitorCfg, err = config.LoadMonitorConfig()
		if err != nil {
			return fmt.Errorf("no monitor config specified and default config not found: %w", err)
		}
	}

	dbPath := config.GetDatabasePath()
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("error creating database directory: %w", err)
	}

	db, err := database.New(dbPath)
	if err != nil {
		return fmt.Errorf("error initializing database: %w", err)
	}
	defer db.Close()

	client := telegram.NewClient(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Resolve usernames to IDs and combine sources
	sourceIDs, err := resolveSources(ctx, client, monitorCfg.SourceChannels, monitorCfg.SourceGroups)
	if err != nil {
		return fmt.Errorf("error resolving source channels/groups: %w", err)
	}

	if len(sourceIDs) == 0 {
		return fmt.Errorf("no valid source channels or groups specified in monitor config")
	}

	// Resolve target channel usernames to IDs
	targetIDs, err := resolveTargets(ctx, client, monitorCfg.TargetChannels)
	if err != nil {
		return fmt.Errorf("error resolving target channels: %w", err)
	}

	if len(targetIDs) == 0 {
		return fmt.Errorf("no valid target channels specified in monitor config")
	}

	// Resolve users for status monitoring
	userIDs, err := resolveUsers(ctx, client, monitorCfg.MonitorUsers)
	if err != nil {
		return fmt.Errorf("error resolving users for monitoring: %w", err)
	}

	if len(userIDs) > 0 {
		fmt.Printf("Monitoring status changes for %d users\n", len(userIDs))
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived shutdown signal. Gracefully shutting down...")
		cancel()
	}()

	fmt.Printf("Starting teleslurp monitor...\n")
	fmt.Printf("Monitoring %d sources and forwarding to %d target channels\n", len(sourceIDs), len(targetIDs))

	// For now, use the first target channel. In the future, we could support multiple targets
	targetChannelID := targetIDs[0]

	if len(userIDs) > 0 {
		return client.MonitorAndForwardWithUsers(ctx, sourceIDs, targetChannelID, userIDs, db)
	} else {
		return client.MonitorAndForward(ctx, sourceIDs, targetChannelID, db)
	}
}
