package telegram

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/gnomegl/teleslurp/internal/config"
	"github.com/gnomegl/teleslurp/internal/types"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

type MessageData struct {
	ChannelTitle string    `json:"channel_title"`
	ChannelUsername string `json:"channel_username"`
	MessageID    int      `json:"message_id"`
	Date         string   `json:"date"`
	Message      string   `json:"message"`
	URL          string   `json:"url"`
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

	headers := []string{"Channel Title", "Channel Username", "Message ID", "Date", "Message", "URL"}
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

		resolvedUser, err := api.ContactsResolveUsername(ctx, searchUser.Username)
		if err != nil {
			return fmt.Errorf("Error resolving username: %v\n", err)
		}

		var targetUser *tg.User
		for _, u := range resolvedUser.Users {
			if tgUser, ok := u.(*tg.User); ok && tgUser.Username == searchUser.Username {
				targetUser = tgUser
				break
			}
		}

		if targetUser == nil {
			return fmt.Errorf("Could not find user information")
		}

		fmt.Printf("Found user %s with ID: %d and AccessHash: %d\n",
			targetUser.Username, targetUser.ID, targetUser.AccessHash)

		var allMessages []MessageData
		for _, group := range groups {
			resolvedPeer, err := api.ContactsResolveUsername(ctx, group.Username)
			if err != nil {
				continue
			}

			channel, ok := resolvedPeer.Chats[0].(*tg.Channel)
			if !ok {
				continue
			}

			offset := 0
			matchCount := 0
			for {
				req := &tg.MessagesSearchRequest{
					Peer: &tg.InputPeerChannel{
						ChannelID:  channel.ID,
						AccessHash: channel.AccessHash,
					},
					Q:      "",
					Filter: &tg.InputMessagesFilterEmpty{},
					FromID: &tg.InputPeerUser{
						UserID:     targetUser.ID,
						AccessHash: targetUser.AccessHash,
					},
					MinDate:   0,
					MaxDate:   int(time.Now().Unix()),
					OffsetID:  0,
					AddOffset: offset,
					Limit:     1000,
					MaxID:     0,
					MinID:     0,
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
						messageURL := fmt.Sprintf("https://t.me/%s/%d", group.Username, m.ID)
						messageData := MessageData{
							ChannelTitle: group.Title,
							ChannelUsername: group.Username,
							MessageID: m.ID,
							Date: time.Unix(int64(m.Date), 0).Format("2006-01-02 15:04:05"),
							Message: m.Message,
							URL: messageURL,
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

		return nil
	})
}
