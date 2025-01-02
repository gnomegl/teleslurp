package telegram

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gnomegl/teleslurp/internal/config"
	"github.com/gnomegl/teleslurp/internal/types"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/schollz/progressbar/v3"
)

type MessageData struct {
	ChannelTitle    string `json:"channel_title"`
	ChannelUsername string `json:"channel_username"`
	MessageID       int    `json:"message_id"`
	Date            string `json:"date"`
	Message         string `json:"message"`
	URL             string `json:"url"`
}

type ChannelMetadata struct {
	ChannelTitle     string `json:"channel_title"`
	ChannelUsername  string `json:"channel_username"`
	ChannelLink      string `json:"channel_link"`
	ChannelAdmins    string `json:"channel_admins"`
	MemberCount      int    `json:"member_count"`
	UserFirstMessage string `json:"user_first_message"`
}

func exportMessagesToJSON(messages []MessageData, username string) error {
	filename := fmt.Sprintf("%s_messages.json", username)
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("error creating JSON file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(messages); err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}

	fmt.Printf("Messages exported to JSON file: %s\n", filename)
	return nil
}

func exportMessagesToCSV(messages []MessageData, username string) error {
	filename := fmt.Sprintf("%s_messages.csv", username)
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("error creating CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	headers := []string{
		"Channel Title",
		"Channel Username",
		"Message ID",
		"Date",
		"Message",
		"URL",
	}
	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("error writing CSV headers: %w", err)
	}

	for _, msg := range messages {
		record := []string{
			msg.ChannelTitle,
			msg.ChannelUsername,
			fmt.Sprintf("%d", msg.MessageID),
			msg.Date,
			msg.Message,
			msg.URL,
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("error writing CSV record: %w", err)
		}
	}

	fmt.Printf("Messages exported to CSV file: %s\n", filename)
	return nil
}

func exportChannelMetadataToJSON(metadata []ChannelMetadata, username string) error {
	filename := fmt.Sprintf("%s_channel_metadata.json", username)
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("error creating JSON file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(metadata); err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}

	fmt.Printf("Channel metadata exported to JSON file: %s\n", filename)
	return nil
}

func exportChannelMetadataToCSV(metadata []ChannelMetadata, username string) error {
	filename := fmt.Sprintf("%s_channel_metadata.csv", username)
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("error creating CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	headers := []string{
		"Channel Title",
		"Channel Username",
		"Channel Link",
		"Channel Admins",
		"Member Count",
		"User Join Date",
	}
	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("error writing CSV headers: %w", err)
	}

	for _, ch := range metadata {
		record := []string{
			ch.ChannelTitle,
			ch.ChannelUsername,
			ch.ChannelLink,
			ch.ChannelAdmins,
			fmt.Sprintf("%d", ch.MemberCount),
			ch.UserFirstMessage,
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("error writing CSV record: %w", err)
		}
	}

	fmt.Printf("Channel metadata exported to CSV file: %s\n", filename)
	return nil
}

type OutputFormat string

const (
	FormatJSON OutputFormat = "json"
	FormatCSV  OutputFormat = "csv"
)

func RunClient(ctx context.Context, cfg *config.Config, searchUser *types.User, groups []types.Group, format OutputFormat) error {
	sessionStore := &session.FileStorage{Path: config.GetSessionPath()}

	client := telegram.NewClient(cfg.TGAPIID, cfg.TGAPIHash, telegram.Options{
		SessionStorage: sessionStore,
	})

	return client.Run(ctx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("failed to get auth status: %w", err)
		}

		if !status.Authorized {
			if cfg.PhoneNumber == "" {
				fmt.Print("Enter your phone number (including country code): ")
				fmt.Scanln(&cfg.PhoneNumber)
				if err := config.Save(cfg); err != nil {
					return fmt.Errorf("failed to save config: %w", err)
				}
			}

			var password string
			if !status.Authorized {
				fmt.Print("Enter your 2FA password (press Enter if none): ")
				fmt.Scanln(&password)
			}

			flow := auth.NewFlow(
				auth.Constant(
					cfg.PhoneNumber,
					password,
					auth.CodeAuthenticatorFunc(
						func(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
							fmt.Print("Enter the code sent to your device: ")
							var code string
							fmt.Scanln(&code)
							return code, nil
						},
					),
				),
				auth.SendCodeOptions{},
			)

			if err := client.Auth().IfNecessary(ctx, flow); err != nil {
				return fmt.Errorf("failed to authenticate: %w", err)
			}
		}

		api := client.API()

		// Convert username to tg id if needed
		var userID int64
		var userAccessHash int64

		if searchUser.Username != "" {
			resolvedUser, err := api.ContactsResolveUsername(ctx, searchUser.Username)
			if err != nil {
				return fmt.Errorf("error resolving username: %w", err)
			}

			for _, u := range resolvedUser.Users {
				if tgUser, ok := u.(*tg.User); ok && tgUser.Username == searchUser.Username {
					userID = tgUser.ID
					userAccessHash = tgUser.AccessHash
					break
				}
			}
			if userID == 0 {
				return fmt.Errorf("could not find user with username: %s", searchUser.Username)
			}
		} else {
			userID = searchUser.ID
			// for id based searches, we try to use a minimal access hash
			// (this is because we don't have access to the real access hash without being in a shared channel)

			// TODO: add a way to get the real access hash, e.g. by using a shared channel like joining
			// and then leaving once we have the access hash
			userAccessHash = userID
		}

		fmt.Printf("Searching messages for user ID: %d\n", userID)

		var allMessages []MessageData
		var allMetadata []ChannelMetadata

		// Initialize progress bar for channel search
		bar := progressbar.NewOptions(len(groups),
			progressbar.OptionSetDescription("Searching channels"),
			progressbar.OptionSetWidth(30),
			progressbar.OptionShowCount(),
			progressbar.OptionSetPredictTime(false),
			progressbar.OptionSetElapsedTime(true),
			progressbar.OptionSetRenderBlankState(true),
			progressbar.OptionThrottle(100*time.Millisecond),
			progressbar.OptionUseANSICodes(true),
			progressbar.OptionEnableColorCodes(true),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "━",
				SaucerHead:    "▶",
				SaucerPadding: "─",
				BarStart:      "[",
				BarEnd:        "]",
			}),
		)

		for groupIdx, group := range groups {
			// Move cursor up one line and clear it
			fmt.Print("\033[1A\033[K")
			fmt.Printf("[%d/%d] Checking %s...\n", groupIdx+1, len(groups), group.Title)

			// convert channel username to ID
			var channelID int64
			var channelAccessHash int64

			if group.Username != "" {
				resolvedPeer, err := api.ContactsResolveUsername(ctx, group.Username)
				if err != nil {
					fmt.Printf("Could not find channel %s\n", group.Username)
					continue
				}

				if len(resolvedPeer.Chats) == 0 {
					fmt.Printf("Could not find channel %s\n", group.Username)
					continue
				}

				channel, ok := resolvedPeer.Chats[0].(*tg.Channel)
				if !ok {
					fmt.Printf("Could not find channel %s\n", group.Username)
					continue
				}
				channelID = channel.ID
				channelAccessHash = channel.AccessHash
			} else {
				channelID = group.ID
				// For channel IDs, we can try using the ID as access hash
				channelAccessHash = group.ID
			}

			chatsResult, err := api.ChannelsGetChannels(ctx, []tg.InputChannelClass{
				&tg.InputChannel{
					ChannelID:  channelID,
					AccessHash: channelAccessHash,
				},
			})
			if err != nil {
				continue
			}

			var channelTitle, channelUsername string
			var memberCount int
			var channelAdmins []string

			var chats []tg.ChatClass
			switch result := chatsResult.(type) {
			case *tg.MessagesChats:
				chats = result.Chats
			case *tg.MessagesChatsSlice:
				chats = result.Chats
			default:
				continue
			}

			// Get channel info
			for _, chat := range chats {
				if channel, ok := chat.(*tg.Channel); ok {
					channelTitle = channel.Title
					channelUsername = channel.Username

					// Get full channel info to ensure we have participant count
					fullChannel, err := api.ChannelsGetFullChannel(ctx, &tg.InputChannel{
						ChannelID:  channelID,
						AccessHash: channelAccessHash,
					})
					if err == nil {
						if fc, ok := fullChannel.FullChat.(*tg.ChannelFull); ok {
							memberCount = fc.ParticipantsCount
						}
					}

					// Try to get admin info
					admins, err := api.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
						Channel: &tg.InputChannel{
							ChannelID:  channelID,
							AccessHash: channelAccessHash,
						},
						Filter: &tg.ChannelParticipantsAdmins{},
						Offset: 0,
						Limit:  100,
					})
					if err == nil {
						if participants, ok := admins.(*tg.ChannelsChannelParticipants); ok {
							for _, user := range participants.Users {
								if u, ok := user.(*tg.User); ok {
									admin := u.Username
									if admin == "" {
										admin = fmt.Sprintf("%s %s", u.FirstName, u.LastName)
									}
									channelAdmins = append(channelAdmins, admin)
								}
							}
						}
					}
					break
				}
			}

			var firstMessageDate time.Time
			var messages []MessageData

			offset := 0
			matchCount := 0
			for {
				req := &tg.MessagesSearchRequest{
					Peer: &tg.InputPeerChannel{
						ChannelID:  channelID,
						AccessHash: channelAccessHash,
					},
					Q:      "",
					Filter: &tg.InputMessagesFilterEmpty{},
					FromID: &tg.InputPeerUser{
						UserID:     userID,
						AccessHash: userAccessHash,
					},
					MaxID:     0,
					MinID:     0,
					MinDate:   0,
					MaxDate:   int(time.Now().Unix()),
					AddOffset: offset,
					Limit:     100,
					Hash:      0,
				}

				result, err := api.MessagesSearch(ctx, req)
				if err != nil {
					fmt.Printf("Error getting message count in %s: %v\n", channelTitle, err)
					break
				}

				msgs, ok := result.(*tg.MessagesChannelMessages)
				if !ok {
					fmt.Printf("Unexpected response type for %s\n", channelTitle)
					break
				}

				if len(msgs.Messages) == 0 {
					break
				}

				for _, msg := range msgs.Messages {
					if m, ok := msg.(*tg.Message); ok {
						matchCount++
						messageURL := fmt.Sprintf("https://t.me/%s/%d", channelUsername, m.ID)
						if channelUsername == "" {
							messageURL = fmt.Sprintf("https://t.me/c/%d/%d", channelID, m.ID)
						}

						messageDate := time.Unix(int64(m.Date), 0)
						if firstMessageDate.IsZero() || messageDate.Before(firstMessageDate) {
							firstMessageDate = messageDate
						}

						messageData := MessageData{
							ChannelTitle:    channelTitle,
							ChannelUsername: channelUsername,
							MessageID:       m.ID,
							Date:            messageDate.Format("2006-01-02 15:04:05"),
							Message:         m.Message,
							URL:             messageURL,
						}
						messages = append(messages, messageData)
					}
				}

				offset += len(msgs.Messages)
				time.Sleep(500 * time.Millisecond)

				if len(msgs.Messages) < 100 {
					break
				}
			}

			if matchCount > 0 {
				// Move cursor up one line and clear it
				fmt.Print("\033[1A\033[K")
				fmt.Printf("Found %d messages in %s\n", matchCount, channelTitle)
			}

			allMessages = append(allMessages, messages...)

			if matchCount > 0 {
				// After collecting all messages, add the metadata
				userFirstMessage := ""
				if !firstMessageDate.IsZero() {
					userFirstMessage = firstMessageDate.Format("2006-01-02 15:04:05")
				}

				channelLink := ""
				if channelUsername != "" {
					channelLink = fmt.Sprintf("https://t.me/%s", channelUsername)
				} else {
					channelLink = fmt.Sprintf("https://t.me/c/%d", channelID)
				}

				metadata := ChannelMetadata{
					ChannelTitle:     channelTitle,
					ChannelUsername:  channelUsername,
					ChannelLink:      channelLink,
					ChannelAdmins:    strings.Join(channelAdmins, ", "),
					MemberCount:      memberCount,
					UserFirstMessage: userFirstMessage,
				}
				allMetadata = append(allMetadata, metadata)
			}

			bar.Add(1)
			time.Sleep(2 * time.Second)
		}

		// Print summary after all channels are processed
		fmt.Printf("\n\nSummary of channels with messages:\n")
		fmt.Printf("================================\n")
		
		var totalMessages int
		var totalMembers int
		for _, metadata := range allMetadata {
			isAdmin := false
			adminList := strings.Split(metadata.ChannelAdmins, ", ")
			for _, admin := range adminList {
				if admin == searchUser.Username {
					isAdmin = true
					break
				}
			}

			channelInfo := fmt.Sprintf("%s (@%s)", metadata.ChannelTitle, metadata.ChannelUsername)
			if metadata.ChannelUsername == "" {
				channelInfo = fmt.Sprintf("%s (%s)", metadata.ChannelTitle, metadata.ChannelLink)
			}

			messageCount := 0
			for _, msg := range allMessages {
				if msg.ChannelUsername == metadata.ChannelUsername {
					messageCount++
				}
			}
			totalMessages += messageCount
			totalMembers += metadata.MemberCount

			if isAdmin {
				fmt.Printf("\033[31m%s\n", channelInfo) // Red color for admin channels
				fmt.Printf("  • Admin Status: Yes\033[0m\n")
			} else {
				fmt.Printf("%s\n", channelInfo)
			}
			fmt.Printf("  • Messages: %d\n", messageCount)
			fmt.Printf("  • Members: %d\n", metadata.MemberCount)
			fmt.Printf("  • First message: %s\n", metadata.UserFirstMessage)
			fmt.Printf("  • Link: %s\n\n", metadata.ChannelLink)
		}

		if len(allMetadata) > 0 {
			fmt.Printf("Total Statistics:\n")
			fmt.Printf("================\n")
			fmt.Printf("Channels with messages: %d\n", len(allMetadata))
			fmt.Printf("Total messages found: %d\n", totalMessages)
			fmt.Printf("Total members in channels: %d\n", totalMembers)
			avgMessagesPerChannel := float64(totalMessages) / float64(len(allMetadata))
			fmt.Printf("Average messages per channel: %.1f\n", avgMessagesPerChannel)
		} else {
			fmt.Printf("\nNo messages found in any channels.\n")
		}

		if len(allMessages) > 0 {
			switch format {
			case FormatJSON:
				if err := exportMessagesToJSON(allMessages, searchUser.Username); err != nil {
					fmt.Printf("Warning: Failed to export messages to JSON: %v\n", err)
				}
			case FormatCSV:
				if err := exportMessagesToCSV(allMessages, searchUser.Username); err != nil {
					fmt.Printf("Warning: Failed to export messages to CSV: %v\n", err)
				}
			default:
				return fmt.Errorf("unsupported output format: %s", format)
			}
		}

		if len(allMetadata) > 0 {
			switch format {
			case FormatJSON:
				if err := exportChannelMetadataToJSON(allMetadata, searchUser.Username); err != nil {
					fmt.Printf("Warning: Failed to export channel metadata to JSON: %v\n", err)
				}
			case FormatCSV:
				if err := exportChannelMetadataToCSV(allMetadata, searchUser.Username); err != nil {
					fmt.Printf("Warning: Failed to export channel metadata to CSV: %v\n", err)
				}
			default:
				return fmt.Errorf("unsupported output format: %s", format)
			}
		}

		return nil
	})
}
