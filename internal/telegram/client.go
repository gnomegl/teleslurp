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

		// Convert username to ID if needed
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
			// TODO: add a way to get the real access hash
			userAccessHash = userID
		}

		fmt.Printf("Searching messages for user ID: %d\n", userID)

		var allMessages []MessageData
		var allMetadata []ChannelMetadata
		for _, group := range groups {
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

			// Get channel info
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

			// Get messages
			messages, err := api.MessagesSearch(ctx, &tg.MessagesSearchRequest{
				Peer: &tg.InputPeerChannel{
					ChannelID:  channelID,
					AccessHash: channelAccessHash,
				},
				Q:         "",
				Filter:    &tg.InputMessagesFilterEmpty{},
				MinDate:   0,
				MaxDate:   int(time.Now().Unix()),
				OffsetID:  0,
				AddOffset: 0,
				Limit:     100,
				MaxID:     0,
				MinID:     0,
				FromID: &tg.InputPeerUser{
					UserID:     userID,
					AccessHash: userAccessHash,
				},
				Hash: 0,
			})

			if err != nil {
				continue
			}

			switch m := messages.(type) {
			case *tg.MessagesChannelMessages:
				for _, msg := range m.Messages {
					if message, ok := msg.(*tg.Message); ok {
						messageURL := fmt.Sprintf("https://t.me/%s/%d", channelUsername, message.ID)
						if channelUsername == "" {
							messageURL = fmt.Sprintf("https://t.me/c/%d/%d", channelID, message.ID)
						}

						messageDate := time.Unix(int64(message.Date), 0)
						if firstMessageDate.IsZero() || messageDate.Before(firstMessageDate) {
							firstMessageDate = messageDate
						}

						messageData := MessageData{
							ChannelTitle:    channelTitle,
							ChannelUsername: channelUsername,
							MessageID:       message.ID,
							Date:            messageDate.Format("2006-01-02 15:04:05"),
							Message:         message.Message,
							URL:             messageURL,
						}
						allMessages = append(allMessages, messageData)
					}
				}
			}

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
					fmt.Printf("Error searching messages in %s: %v\n", group.Title, err)
					break
				}

				msgs, ok := result.(*tg.MessagesChannelMessages)
				if !ok {
					fmt.Printf("Unexpected response type for %s\n", group.Title)
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
						allMessages = append(allMessages, messageData)

						fmt.Printf("\nMessage #%d:\n", matchCount)
						fmt.Printf("Channel: %s\n", messageData.ChannelTitle)
						fmt.Printf("Date: %s\n", messageData.Date)
						fmt.Printf("Message: %s\n", messageData.Message)
						fmt.Printf("Link: %s\n", messageData.URL)
					}
				}

				offset += len(msgs.Messages)

				time.Sleep(500 * time.Millisecond)

				if len(msgs.Messages) < 100 {
					break
				}
			}

			if matchCount > 0 {
				fmt.Printf("\nFound %d messages from user in %s\n", matchCount, group.Title)
			} else {
				fmt.Printf("No messages found from user in %s\n", group.Title)
			}

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

			time.Sleep(2 * time.Second)
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
