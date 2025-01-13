package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gnomegl/teleslurp/internal/config"
	"github.com/gnomegl/teleslurp/internal/export"
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

type OutputFormat string

const (
	FormatJSON OutputFormat = "json"
	FormatCSV  OutputFormat = "csv"
)

func exportMessagesToJSON(messages []MessageData, username string) error {
	filename := export.FormatFilename(username, "messages", "json")
	return export.WriteJSON(messages, filename)
}

func exportMessagesToCSV(messages []MessageData, username string) error {
	filename := export.FormatFilename(username, "messages", "csv")
	writer, err := export.NewCSVWriter(filename)
	if err != nil {
		return err
	}
	defer writer.Close()

	headers := []string{
		"Channel Title",
		"Channel Username",
		"Message ID",
		"Date",
		"Message",
		"URL",
	}
	if err := writer.WriteHeader(headers); err != nil {
		return err
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
		if err := writer.WriteRecord(record); err != nil {
			return err
		}
	}

	fmt.Printf("Messages exported to CSV file: %s\n", filename)
	return nil
}

func exportChannelMetadataToJSON(metadata []ChannelMetadata, username string) error {
	filename := export.FormatFilename(username, "channel_metadata", "json")
	return export.WriteJSON(metadata, filename)
}

func exportChannelMetadataToCSV(metadata []ChannelMetadata, username string) error {
	filename := export.FormatFilename(username, "channel_metadata", "csv")
	writer, err := export.NewCSVWriter(filename)
	if err != nil {
		return err
	}
	defer writer.Close()

	headers := []string{
		"Channel Title",
		"Channel Username",
		"Channel Link",
		"Channel Admins",
		"Member Count",
		"User Join Date",
	}
	if err := writer.WriteHeader(headers); err != nil {
		return err
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
		if err := writer.WriteRecord(record); err != nil {
			return err
		}
	}

	fmt.Printf("Channel metadata exported to CSV file: %s\n", filename)
	return nil
}

type Client struct {
	cfg    *config.Config
	client *telegram.Client
	api    *tg.Client
}

func NewClient(cfg *config.Config) *Client {
	sessionStore := &session.FileStorage{Path: config.GetSessionPath()}
	client := telegram.NewClient(cfg.TGAPIID, cfg.TGAPIHash, telegram.Options{
		SessionStorage: sessionStore,
	})

	return &Client{
		cfg:    cfg,
		client: client,
	}
}

func (c *Client) authenticate(ctx context.Context) error {
	status, err := c.client.Auth().Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get auth status: %w", err)
	}

	if !status.Authorized {
		if c.cfg.PhoneNumber == "" {
			fmt.Print("Enter your phone number (including country code): ")
			fmt.Scanln(&c.cfg.PhoneNumber)
			if err := config.Save(c.cfg); err != nil {
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
				c.cfg.PhoneNumber,
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

		if err := c.client.Auth().IfNecessary(ctx, flow); err != nil {
			return fmt.Errorf("failed to authenticate: %w", err)
		}
	}

	return nil
}

func (c *Client) resolveUser(ctx context.Context, searchUser *types.User) (int64, int64, error) {
	if searchUser.Username != "" {
		resolvedUser, err := c.api.ContactsResolveUsername(ctx, searchUser.Username)
		if err != nil {
			return 0, 0, fmt.Errorf("error resolving username: %w", err)
		}

		for _, u := range resolvedUser.Users {
			if tgUser, ok := u.(*tg.User); ok && tgUser.Username == searchUser.Username {
				return tgUser.ID, tgUser.AccessHash, nil
			}
		}
		return 0, 0, fmt.Errorf("could not find user with username: %s", searchUser.Username)
	}

	// For ID-based searches, use minimal access hash
	return searchUser.ID, searchUser.ID, nil
}

func (c *Client) searchChannel(ctx context.Context, channel types.Group, userID, userAccessHash int64) (*ChannelSearchResult, error) {
	var channelID int64
	var channelAccessHash int64

	if channel.Username != "" {
		resolvedPeer, err := c.api.ContactsResolveUsername(ctx, channel.Username)
		if err != nil {
			return nil, fmt.Errorf("could not find channel %s: %w", channel.Username, err)
		}

		if len(resolvedPeer.Chats) == 0 {
			return nil, fmt.Errorf("could not find channel %s", channel.Username)
		}

		ch, ok := resolvedPeer.Chats[0].(*tg.Channel)
		if !ok {
			return nil, fmt.Errorf("could not find channel %s", channel.Username)
		}
		channelID = ch.ID
		channelAccessHash = ch.AccessHash
	} else {
		channelID = channel.ID
		channelAccessHash = channel.ID
	}

	result := &ChannelSearchResult{
		ChannelID:  channelID,
		AccessHash: channelAccessHash,
		Messages:   []MessageData{},
	}

	chats, err := c.getChannelInfo(ctx, channelID, channelAccessHash)
	if err != nil {
		return nil, err
	}

	for _, chat := range chats {
		if channel, ok := chat.(*tg.Channel); ok {
			result.Title = channel.Title
			result.Username = channel.Username

			fullChannel, err := c.api.ChannelsGetFullChannel(ctx, &tg.InputChannel{
				ChannelID:  channelID,
				AccessHash: channelAccessHash,
			})
			if err == nil {
				if fc, ok := fullChannel.FullChat.(*tg.ChannelFull); ok {
					result.MemberCount = fc.ParticipantsCount
				}
			}

			admins, err := c.getChannelAdmins(ctx, channelID, channelAccessHash)
			if err == nil {
				result.Admins = admins
			}
			break
		}
	}

	messages, firstMessageDate, err := c.searchMessages(ctx, channelID, channelAccessHash, userID, userAccessHash)
	if err != nil {
		return nil, err
	}
	result.Messages = messages
	result.FirstMessageDate = firstMessageDate

	return result, nil
}

func (c *Client) getChannelInfo(ctx context.Context, channelID, accessHash int64) ([]tg.ChatClass, error) {
	chatsResult, err := c.api.ChannelsGetChannels(ctx, []tg.InputChannelClass{
		&tg.InputChannel{
			ChannelID:  channelID,
			AccessHash: accessHash,
		},
	})
	if err != nil {
		return nil, err
	}

	switch result := chatsResult.(type) {
	case *tg.MessagesChats:
		return result.Chats, nil
	case *tg.MessagesChatsSlice:
		return result.Chats, nil
	default:
		return nil, fmt.Errorf("unexpected response type")
	}
}

func (c *Client) getChannelAdmins(ctx context.Context, channelID, accessHash int64) ([]string, error) {
	admins, err := c.api.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
		Channel: &tg.InputChannel{
			ChannelID:  channelID,
			AccessHash: accessHash,
		},
		Filter: &tg.ChannelParticipantsAdmins{},
		Offset: 0,
		Limit:  100,
	})
	if err != nil {
		return nil, err
	}

	var adminList []string
	if participants, ok := admins.(*tg.ChannelsChannelParticipants); ok {
		for _, user := range participants.Users {
			if u, ok := user.(*tg.User); ok {
				admin := u.Username
				if admin == "" {
					admin = fmt.Sprintf("%s %s", u.FirstName, u.LastName)
				}
				adminList = append(adminList, admin)
			}
		}
	}
	return adminList, nil
}

func (c *Client) searchMessages(ctx context.Context, channelID, channelAccessHash, userID, userAccessHash int64) ([]MessageData, time.Time, error) {
	var messages []MessageData
	var firstMessageDate time.Time
	offset := 0

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

		result, err := c.api.MessagesSearch(ctx, req)
		if err != nil {
			return nil, firstMessageDate, fmt.Errorf("error searching messages: %w", err)
		}

		msgs, ok := result.(*tg.MessagesChannelMessages)
		if !ok {
			return nil, firstMessageDate, fmt.Errorf("unexpected response type")
		}

		if len(msgs.Messages) == 0 {
			break
		}

		for _, msg := range msgs.Messages {
			if m, ok := msg.(*tg.Message); ok {
				messageDate := time.Unix(int64(m.Date), 0)
				if firstMessageDate.IsZero() || messageDate.Before(firstMessageDate) {
					firstMessageDate = messageDate
				}

				messageURL := formatMessageURL(channelID, m.ID, msgs.Chats[0].(*tg.Channel).Username)
				messages = append(messages, MessageData{
					MessageID: m.ID,
					Date:      messageDate.Format("2006-01-02 15:04:05"),
					Message:   m.Message,
					URL:       messageURL,
				})
			}
		}

		offset += len(msgs.Messages)
		time.Sleep(500 * time.Millisecond)

		if len(msgs.Messages) < 100 {
			break
		}
	}

	return messages, firstMessageDate, nil
}

func formatMessageURL(channelID int64, messageID int, username string) string {
	if username != "" {
		return fmt.Sprintf("https://t.me/%s/%d", username, messageID)
	}
	return fmt.Sprintf("https://t.me/c/%d/%d", channelID, messageID)
}

type ChannelSearchResult struct {
	ChannelID        int64
	AccessHash       int64
	Title            string
	Username         string
	MemberCount      int
	Admins           []string
	Messages         []MessageData
	FirstMessageDate time.Time
}

func (c *Client) Run(ctx context.Context, searchUser *types.User, groups []types.Group, format OutputFormat, exportMetadata bool) error {
	if err := c.client.Run(ctx, func(ctx context.Context) error {
		if err := c.authenticate(ctx); err != nil {
			return err
		}

		c.api = c.client.API()
		userID, userAccessHash, err := c.resolveUser(ctx, searchUser)
		if err != nil {
			return err
		}

		fmt.Printf("Searching messages for user ID: %d\n", userID)

		var allMessages []MessageData
		var allMetadata []ChannelMetadata

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
			fmt.Print("\033[1A\033[K")
			fmt.Printf("[%d/%d] Checking %s...\n", groupIdx+1, len(groups), group.Title)

			result, err := c.searchChannel(ctx, group, userID, userAccessHash)
			if err != nil {
				fmt.Printf("Error searching channel: %v\n", err)
				continue
			}

			if result == nil {
				continue
			}

			if len(result.Messages) > 0 {
				fmt.Print("\033[1A\033[K")
				fmt.Printf("Found %d messages in %s\n", len(result.Messages), result.Title)

				allMessages = append(allMessages, result.Messages...)
				allMetadata = append(allMetadata, ChannelMetadata{
					ChannelTitle:     result.Title,
					ChannelUsername:  result.Username,
					ChannelLink:      formatMessageURL(result.ChannelID, 0, result.Username),
					ChannelAdmins:    strings.Join(result.Admins, ", "),
					MemberCount:      result.MemberCount,
					UserFirstMessage: result.FirstMessageDate.Format("2006-01-02 15:04:05"),
				})
			}

			bar.Add(1)
			time.Sleep(2 * time.Second)
		}

		if len(allMessages) == 0 {
			fmt.Println("No messages found")
			return nil
		}

		if err := c.printSummary(allMetadata, allMessages, searchUser); err != nil {
			return err
		}

		return c.exportResults(allMessages, allMetadata, searchUser.Username, format, exportMetadata)
	}); err != nil {
		return fmt.Errorf("error running client: %w", err)
	}

	return nil
}

func (c *Client) printSummary(metadata []ChannelMetadata, messages []MessageData, searchUser *types.User) error {
	fmt.Printf("\n\nSummary of channels with messages:\n")
	fmt.Printf("================================\n")

	var totalMessages int
	var totalMembers int
	for _, meta := range metadata {
		isAdmin := false
		adminList := strings.Split(meta.ChannelAdmins, ", ")
		for _, admin := range adminList {
			if admin == searchUser.Username {
				isAdmin = true
				break
			}
		}

		channelInfo := fmt.Sprintf("%s (@%s)", meta.ChannelTitle, meta.ChannelUsername)
		if meta.ChannelUsername == "" {
			channelInfo = fmt.Sprintf("%s (%s)", meta.ChannelTitle, meta.ChannelLink)
		}

		messageCount := 0
		for _, msg := range messages {
			if msg.ChannelUsername == meta.ChannelUsername {
				messageCount++
			}
		}
		totalMessages += messageCount
		totalMembers += meta.MemberCount

		if isAdmin {
			fmt.Printf("\033[31m%s\n", channelInfo)
			fmt.Printf("  • Admin Status: Yes\033[0m\n")
		} else {
			fmt.Printf("%s\n", channelInfo)
		}
		fmt.Printf("  • Messages: %d\n", messageCount)
		fmt.Printf("  • Members: %d\n", meta.MemberCount)
		fmt.Printf("  • First message: %s\n", meta.UserFirstMessage)
		fmt.Printf("  • Link: %s\n\n", meta.ChannelLink)
	}

	if len(metadata) > 0 {
		fmt.Printf("Total Statistics:\n")
		fmt.Printf("================\n")
		fmt.Printf("Channels with messages: %d\n", len(metadata))
		fmt.Printf("Total messages found: %d\n", totalMessages)
		fmt.Printf("Total members in channels: %d\n", totalMembers)
		avgMessagesPerChannel := float64(totalMessages) / float64(len(metadata))
		fmt.Printf("Average messages per channel: %.1f\n", avgMessagesPerChannel)
	} else {
		fmt.Printf("\nNo messages found in any channels.\n")
	}

	return nil
}

func (c *Client) exportResults(messages []MessageData, metadata []ChannelMetadata, username string, format OutputFormat, exportMetadata bool) error {
	switch format {
	case FormatJSON:
		if err := exportMessagesToJSON(messages, username); err != nil {
			fmt.Printf("Warning: Failed to export messages to JSON: %v\n", err)
		}
		if exportMetadata {
			if err := exportChannelMetadataToJSON(metadata, username); err != nil {
				fmt.Printf("Warning: Failed to export channel metadata to JSON: %v\n", err)
			}
		}
	case FormatCSV:
		if err := exportMessagesToCSV(messages, username); err != nil {
			fmt.Printf("Warning: Failed to export messages to CSV: %v\n", err)
		}
		if exportMetadata {
			if err := exportChannelMetadataToCSV(metadata, username); err != nil {
				fmt.Printf("Warning: Failed to export channel metadata to CSV: %v\n", err)
			}
		}
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}

	return nil
}

func RunClient(ctx context.Context, cfg *config.Config, searchUser *types.User, groups []types.Group, format OutputFormat, exportMetadata bool) error {
	client := NewClient(cfg)
	return client.Run(ctx, searchUser, groups, format, exportMetadata)
}
