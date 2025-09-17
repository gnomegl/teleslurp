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
	"math/rand"
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
	opts := telegram.Options{
		NoUpdates:      false,
		SessionStorage: sessionStore,
	}

	client := telegram.NewClient(cfg.TGAPIID, cfg.TGAPIHash, opts)
	return &Client{
		cfg:    cfg,
		client: client,
		api:    client.API(),
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
				searchUser.ID = tgUser.ID
				searchUser.Username = tgUser.Username
				return tgUser.ID, tgUser.AccessHash, nil
			}
		}
		return 0, 0, fmt.Errorf("could not find user with username: %s", searchUser.Username)
	}

	// For ID-based searches, we'll try to resolve username from group participants later
	return searchUser.ID, searchUser.ID, nil
}

func (c *Client) tryResolveUsernameFromGroups(ctx context.Context, userID int64, groups []types.Group) (string, int64) {
	for _, group := range groups {
		var channelID int64
		var channelAccessHash int64

		if group.Username != "" {
			cleanUsername := strings.TrimPrefix(group.Username, "@")
			resolvedPeer, err := c.api.ContactsResolveUsername(ctx, cleanUsername)
			if err != nil {
				continue
			}

			for _, chat := range resolvedPeer.Chats {
				if ch, ok := chat.(*tg.Channel); ok {
					channelID = ch.ID
					channelAccessHash = ch.AccessHash
					break
				}
			}
		} else if group.ID != 0 {
			channelID = group.ID
			channelAccessHash = group.ID
		}

		if channelID == 0 {
			continue
		}

		// Try to get participants to find the user
		participants, err := c.api.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
			Channel: &tg.InputChannel{
				ChannelID:  channelID,
				AccessHash: channelAccessHash,
			},
			Filter: &tg.ChannelParticipantsSearch{
				Q: "",
			},
			Offset: 0,
			Limit:  200,
		})
		if err != nil {
			continue
		}

		if channelParticipants, ok := participants.(*tg.ChannelsChannelParticipants); ok {
			for _, user := range channelParticipants.Users {
				if u, ok := user.(*tg.User); ok && u.ID == userID {
					// Found the user! Return their current username and access hash
					return u.Username, u.AccessHash
				}
			}
		}
	}

	return "", 0
}

func (c *Client) searchChannel(ctx context.Context, channel types.Group, userID, userAccessHash int64, searchUser *types.User) (*ChannelSearchResult, error) {
	var channelID int64
	var channelAccessHash int64

	if channel.Username != "" {
		cleanUsername := strings.TrimPrefix(channel.Username, "@")

		resolvedPeer, err := c.api.ContactsResolveUsername(ctx, cleanUsername)
		if err != nil {
			if strings.Contains(err.Error(), "USERNAME_NOT_OCCUPIED") || strings.Contains(err.Error(), "USERNAME_INVALID") {
				return nil, fmt.Errorf("channel %s not found (may be private or renamed)", cleanUsername)
			}
			return nil, fmt.Errorf("could not find channel %s: %w", cleanUsername, err)
		}

		if len(resolvedPeer.Chats) == 0 {
			return nil, fmt.Errorf("could not find channel %s", cleanUsername)
		}

		for _, chat := range resolvedPeer.Chats {
			if ch, ok := chat.(*tg.Channel); ok {
				channelID = ch.ID
				channelAccessHash = ch.AccessHash
				break
			}
		}

		if channelID == 0 {
			return nil, fmt.Errorf("could not find channel %s", cleanUsername)
		}
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

	if searchUser != nil && searchUser.Username == "" && searchUser.ID != 0 {
		participants, err := c.api.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
			Channel: &tg.InputChannel{
				ChannelID:  channelID,
				AccessHash: channelAccessHash,
			},
			Filter: &tg.ChannelParticipantsSearch{
				Q: "",
			},
			Offset: 0,
			Limit:  200,
		})
		if err == nil {
			if channelParticipants, ok := participants.(*tg.ChannelsChannelParticipants); ok {
				for _, user := range channelParticipants.Users {
					if u, ok := user.(*tg.User); ok && u.ID == userID {
						// Found the user! Update their username
						searchUser.Username = u.Username
						if u.AccessHash != 0 {
							userAccessHash = u.AccessHash
						}
						break
					}
				}
			}
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

				var channelUsername string
				if len(msgs.Chats) > 0 {
					if ch, ok := msgs.Chats[0].(*tg.Channel); ok {
						channelUsername = ch.Username
					}
				}
				messageURL := formatMessageURL(channelID, m.ID, channelUsername)
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

		if searchUser.ID != 0 && searchUser.Username == "" {
			resolvedUsername, resolvedAccessHash := c.tryResolveUsernameFromGroups(ctx, userID, groups)
			if resolvedUsername != "" {
				searchUser.Username = resolvedUsername
				if resolvedAccessHash != 0 {
					userAccessHash = resolvedAccessHash
				}
				fmt.Printf("Resolved username for ID %d: @%s\n", userID, resolvedUsername)
			}
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

			result, err := c.searchChannel(ctx, group, userID, userAccessHash, searchUser)
			if err != nil {
				// More detailed error message
				if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "USERNAME") {
					fmt.Printf("Channel %s not accessible (private/renamed/deleted)\n", group.Title)
				} else {
					fmt.Printf("Error searching channel: %v\n", err)
				}
				continue
			}

			if result == nil {
				continue
			}

			if len(result.Messages) > 0 {
				fmt.Print("\033[1A\033[K")
				fmt.Printf("Found %d messages in %s\n", len(result.Messages), result.Title)

				for i := range result.Messages {
					result.Messages[i].ChannelTitle = result.Title
					result.Messages[i].ChannelUsername = result.Username
				}

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
			if msg.ChannelUsername == meta.ChannelUsername || (meta.ChannelUsername == "" && msg.ChannelTitle == meta.ChannelTitle) {
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

func (c *Client) GetChannelMessages(ctx context.Context, channelID int64) ([]MessageData, error) {
	if err := c.authenticate(ctx); err != nil {
		return nil, fmt.Errorf("error authenticating: %w", err)
	}

	channel, err := c.api.ChannelsGetFullChannel(ctx, &tg.InputChannel{
		ChannelID:  channelID,
		AccessHash: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("error getting channel: %w", err)
	}

	messages := make([]MessageData, 0)
	channelInfo := channel.Chats[0]
	var channelTitle string
	if ch, ok := channelInfo.(*tg.Channel); ok {
		channelTitle = ch.Title
	}

	history, err := c.api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer: &tg.InputPeerChannel{
			ChannelID:  channelID,
			AccessHash: 0,
		},
		Limit: 100, // Fetch last 100 messages
	})
	if err != nil {
		return nil, fmt.Errorf("error getting messages: %w", err)
	}

	msgs := history.(*tg.MessagesChannelMessages)
	for _, msg := range msgs.Messages {
		message, ok := msg.(*tg.Message)
		if !ok || message.Message == "" {
			continue
		}

		messages = append(messages, MessageData{
			ChannelTitle: channelTitle,
			MessageID:    message.ID,
			Date:         time.Unix(int64(message.Date), 0).Format(time.RFC3339),
			Message:      message.Message,
			URL:          formatMessageURL(channelID, message.ID, msgs.Chats[0].(*tg.Channel).Username),
		})
	}

	return messages, nil
}

func (c *Client) GetChannelsMessages(ctx context.Context, channelIDs []int64) ([]MessageData, error) {
	var allMessages []MessageData
	for _, channelID := range channelIDs {
		messages, err := c.GetChannelMessages(ctx, channelID)
		if err != nil {
			fmt.Printf("Error getting messages for channel %d: %v\n", channelID, err)
			continue
		}
		allMessages = append(allMessages, messages...)
	}
	return allMessages, nil
}

func (c *Client) MonitorChannels(ctx context.Context, channelIDs []int64, handler func(MessageData) error) error {
	if err := c.authenticate(ctx); err != nil {
		return fmt.Errorf("error authenticating: %w", err)
	}

	channels := make(map[int64]bool)
	for _, id := range channelIDs {
		channels[id] = true
	}

	d := tg.NewUpdateDispatcher()

	// Register handler for new channel messages
	d.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewChannelMessage) error {
		fmt.Println("Received new channel message update")

		msg, ok := update.Message.(*tg.Message)
		if !ok {
			fmt.Printf("Update message is not *tg.Message, got: %T\n", update.Message)
			return nil
		}
		fmt.Printf("Message content: %s\n", msg.Message)

		// Check if this is from a monitored channel
		peer, ok := msg.PeerID.(*tg.PeerChannel)
		if !ok {
			fmt.Printf("Message peer is not a channel, got: %T\n", msg.PeerID)
			return nil
		}
		channelID := peer.ChannelID
		if !channels[channelID] {
			fmt.Printf("Message from unmonitored channel: %d\n", channelID)
			return nil
		}
		fmt.Printf("Message is from monitored channel: %d\n", channelID)

		// Get channel info
		fmt.Printf("Getting channel info for: %d\n", channelID)
		channel, err := c.api.ChannelsGetFullChannel(ctx, &tg.InputChannel{
			ChannelID:  channelID,
			AccessHash: 0,
		})
		if err != nil {
			fmt.Printf("Error getting channel info: %v\n", err)
			return nil
		}
		fmt.Println("Successfully got channel info")

		channelInfo := channel.Chats[0]
		var channelTitle string
		if ch, ok := channelInfo.(*tg.Channel); ok {
			channelTitle = ch.Title
			fmt.Printf("Channel title: %s\n", channelTitle)
		}

		// Check if message is from a channel that has forwarding disabled
		isProtected := false
		if channel, ok := channelInfo.(*tg.Channel); ok {
			isProtected = channel.Noforwards
			fmt.Printf("Channel forwarding protection: %v\n", isProtected)
		}

		// If the channel has forwarding disabled, we'll indicate this in the message
		var attribution string
		if isProtected {
			attribution = fmt.Sprintf("\n\n[Protected Content] Originally posted in: %s", channelTitle)
		} else {
			attribution = fmt.Sprintf("\n\nForwarded from: %s", channelTitle)
		}

		// Prepare message text with attribution
		messageText := fmt.Sprintf("%s%s", msg.Message, attribution)
		fmt.Printf("Prepared message text: %s\n", messageText)

		// Create target channel peer
		targetPeer := &tg.InputPeerChannel{
			ChannelID:  channelID,
			AccessHash: 0,
		}
		fmt.Printf("Created target peer for channel: %d\n", channelID)

		// Handle media
		if msg.Media != nil {
			fmt.Printf("Message contains media of type: %T\n", msg.Media)
			switch m := msg.Media.(type) {
			case *tg.MessageMediaPhoto:
				fmt.Println("Processing photo message")
				if isProtected {
					fmt.Println("Photo is from protected channel, sending text-only message")
					_, err = c.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
						Peer:     targetPeer,
						Message:  messageText + "\n[Photo was in original message but cannot be forwarded due to content protection]",
						RandomID: rand.Int63(),
					})
					if err != nil {
						fmt.Printf("Error sending protected photo message: %v\n", err)
						return nil
					}
					fmt.Println("Successfully sent protected photo message")
					break
				}

				fmt.Println("Starting photo download process")
				// Download and reupload photo
				photo := m.Photo.(*tg.Photo)
				largest := photo.Sizes[len(photo.Sizes)-1].(*tg.PhotoSize)

				// Download photo in chunks
				var chunks [][]byte
				offset := 0
				for {
					file, err := c.api.UploadGetFile(ctx, &tg.UploadGetFileRequest{
						Location: &tg.InputPhotoFileLocation{
							ID:            photo.ID,
							AccessHash:    photo.AccessHash,
							FileReference: photo.FileReference,
							ThumbSize:     largest.Type,
						},
						Offset: int64(offset),
						Limit:  524288, // 512KB chunks
					})
					if err != nil {
						fmt.Printf("Error downloading photo chunk: %v\n", err)
						return nil
					}

					data, ok := file.(*tg.UploadFile)
					if !ok {
						fmt.Printf("Unexpected response type for photo download\n")
						return nil
					}

					chunks = append(chunks, data.Bytes)
					offset += len(data.Bytes)

					if len(data.Bytes) < 524288 {
						break
					}
				}

				fmt.Printf("Successfully downloaded photo in %d chunks\n", len(chunks))

				// Upload photo chunks
				fileID := rand.Int63()
				for i, chunk := range chunks {
					uploaded, err := c.api.UploadSaveFilePart(ctx, &tg.UploadSaveFilePartRequest{
						FileID:   fileID,
						FilePart: i,
						Bytes:    chunk,
					})
					if err != nil || !uploaded {
						fmt.Printf("Error uploading photo chunk: %v\n", err)
						return nil
					}
				}

				fmt.Printf("Successfully uploaded photo in %d chunks\n", len(chunks))

				// Send message with photo
				fmt.Println("Sending photo message to target channel")
				_, err = c.api.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
					Peer: targetPeer,
					Media: &tg.InputMediaUploadedPhoto{
						File: &tg.InputFile{
							ID:          fileID,
							Parts:       len(chunks),
							Name:        fmt.Sprintf("photo_%d.jpg", photo.ID),
							MD5Checksum: "",
						},
					},
					Message:  messageText,
					RandomID: rand.Int63(),
				})
				if err != nil {
					fmt.Printf("Error sending photo message: %v\n", err)
					return nil
				}
				fmt.Println("Successfully sent photo message")

			case *tg.MessageMediaDocument:
				fmt.Println("Processing document message")
				// Similar logging for document handling...
				// ...
			default:
				fmt.Printf("Unhandled media type: %T, sending as text-only\n", m)
				// For text-only messages
				_, err = c.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
					Peer:     targetPeer,
					Message:  messageText,
					RandomID: rand.Int63(),
				})
				if err != nil {
					fmt.Printf("Error sending text message: %v\n", err)
					return nil
				}
				fmt.Println("Successfully sent text-only message")
			}
		} else {
			fmt.Println("Message contains no media, sending as text-only")
			// For text-only messages
			_, err = c.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
				Peer:     targetPeer,
				Message:  messageText,
				RandomID: rand.Int63(),
			})
			if err != nil {
				fmt.Printf("Error sending text message: %v\n", err)
				return nil
			}
			fmt.Println("Successfully sent text-only message")
		}

		fmt.Printf("Successfully forwarded message from %s to target channel\n", channelTitle)
		return nil
	})

	fmt.Println("Registered message handler")

	// Now authenticate
	if err := c.authenticate(ctx); err != nil {
		return fmt.Errorf("error authenticating: %w", err)
	}
	fmt.Println("Successfully authenticated")

	// Start receiving updates
	fmt.Println("Starting update loop...")

	// Get initial channel states
	fmt.Println("Getting initial channel states...")
	for channelID := range channels {
		fmt.Printf("Getting initial state for channel %d\n", channelID)
		_, err := c.api.UpdatesGetChannelDifference(ctx, &tg.UpdatesGetChannelDifferenceRequest{
			Channel: &tg.InputChannel{
				ChannelID:  channelID,
				AccessHash: 0,
			},
			Filter: &tg.ChannelMessagesFilterEmpty{},
			Pts:    0,
			Limit:  100,
		})
		if err != nil {
			fmt.Printf("Error getting channel difference for %d: %v\n", channelID, err)
		} else {
			fmt.Printf("Successfully got initial state for channel %d\n", channelID)
		}
	}

	// Run the client to start receiving updates
	return c.client.Run(ctx, func(ctx context.Context) error {
		fmt.Printf("Client running, monitoring %d channels...\n", len(channels))
		<-ctx.Done()
		fmt.Println("Update loop terminated")
		return nil
	})
}

func (c *Client) MonitorAndForward(ctx context.Context, sourceChannelIDs []int64, targetChannelID int64) error {
	fmt.Printf("Starting MonitorAndForward with source channels: %v, target: %d\n", sourceChannelIDs, targetChannelID)

	// Create a map of channel IDs for quick lookup
	channels := make(map[int64]bool)
	for _, id := range sourceChannelIDs {
		channels[id] = true
	}

	// Create a dispatcher and register handlers
	dispatcher := tg.NewUpdateDispatcher()
	fmt.Println("Created update dispatcher")

	// Register handler for new channel messages
	dispatcher.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewChannelMessage) error {
		fmt.Println("Received new channel message update")

		msg, ok := update.Message.(*tg.Message)
		if !ok {
			fmt.Printf("Update message is not *tg.Message, got: %T\n", update.Message)
			return nil
		}
		fmt.Printf("Message content: %s\n", msg.Message)

		// Check if this is from a monitored channel
		peer, ok := msg.PeerID.(*tg.PeerChannel)
		if !ok {
			fmt.Printf("Message peer is not a channel, got: %T\n", msg.PeerID)
			return nil
		}
		channelID := peer.ChannelID
		if !channels[channelID] {
			fmt.Printf("Message from unmonitored channel: %d\n", channelID)
			return nil
		}
		fmt.Printf("Message is from monitored channel: %d\n", channelID)

		// Get channel info
		fmt.Printf("Getting channel info for: %d\n", channelID)
		channel, err := c.api.ChannelsGetFullChannel(ctx, &tg.InputChannel{
			ChannelID:  channelID,
			AccessHash: 0,
		})
		if err != nil {
			fmt.Printf("Error getting channel info: %v\n", err)
			return nil
		}
		fmt.Println("Successfully got channel info")

		channelInfo := channel.Chats[0]
		var channelTitle string
		if ch, ok := channelInfo.(*tg.Channel); ok {
			channelTitle = ch.Title
			fmt.Printf("Channel title: %s\n", channelTitle)
		}

		// Check if message is from a channel that has forwarding disabled
		isProtected := false
		if channel, ok := channelInfo.(*tg.Channel); ok {
			isProtected = channel.Noforwards
			fmt.Printf("Channel forwarding protection: %v\n", isProtected)
		}

		// If the channel has forwarding disabled, we'll indicate this in the message
		var attribution string
		if isProtected {
			attribution = fmt.Sprintf("\n\n[Protected Content] Originally posted in: %s", channelTitle)
		} else {
			attribution = fmt.Sprintf("\n\nForwarded from: %s", channelTitle)
		}

		// Prepare message text with attribution
		messageText := fmt.Sprintf("%s%s", msg.Message, attribution)
		fmt.Printf("Prepared message text: %s\n", messageText)

		// Create target channel peer
		targetPeer := &tg.InputPeerChannel{
			ChannelID:  targetChannelID,
			AccessHash: 0,
		}
		fmt.Printf("Created target peer for channel: %d\n", targetChannelID)

		// Handle media
		if msg.Media != nil {
			fmt.Printf("Message contains media of type: %T\n", msg.Media)
			switch m := msg.Media.(type) {
			case *tg.MessageMediaPhoto:
				fmt.Println("Processing photo message")
				if isProtected {
					fmt.Println("Photo is from protected channel, sending text-only message")
					_, err = c.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
						Peer:     targetPeer,
						Message:  messageText + "\n[Photo was in original message but cannot be forwarded due to content protection]",
						RandomID: rand.Int63(),
					})
					if err != nil {
						fmt.Printf("Error sending protected photo message: %v\n", err)
						return nil
					}
					fmt.Println("Successfully sent protected photo message")
					break
				}

				fmt.Println("Starting photo download process")
				// Download and reupload photo
				photo := m.Photo.(*tg.Photo)
				largest := photo.Sizes[len(photo.Sizes)-1].(*tg.PhotoSize)

				// Download photo in chunks
				var chunks [][]byte
				offset := 0
				for {
					file, err := c.api.UploadGetFile(ctx, &tg.UploadGetFileRequest{
						Location: &tg.InputPhotoFileLocation{
							ID:            photo.ID,
							AccessHash:    photo.AccessHash,
							FileReference: photo.FileReference,
							ThumbSize:     largest.Type,
						},
						Offset: int64(offset),
						Limit:  524288, // 512KB chunks
					})
					if err != nil {
						fmt.Printf("Error downloading photo chunk: %v\n", err)
						return nil
					}

					data, ok := file.(*tg.UploadFile)
					if !ok {
						fmt.Printf("Unexpected response type for photo download\n")
						return nil
					}

					chunks = append(chunks, data.Bytes)
					offset += len(data.Bytes)

					if len(data.Bytes) < 524288 {
						break
					}
				}

				fmt.Printf("Successfully downloaded photo in %d chunks\n", len(chunks))

				// Upload photo chunks
				fileID := rand.Int63()
				for i, chunk := range chunks {
					uploaded, err := c.api.UploadSaveFilePart(ctx, &tg.UploadSaveFilePartRequest{
						FileID:   fileID,
						FilePart: i,
						Bytes:    chunk,
					})
					if err != nil || !uploaded {
						fmt.Printf("Error uploading photo chunk: %v\n", err)
						return nil
					}
				}

				fmt.Printf("Successfully uploaded photo in %d chunks\n", len(chunks))

				// Send message with photo
				fmt.Println("Sending photo message to target channel")
				_, err = c.api.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
					Peer: targetPeer,
					Media: &tg.InputMediaUploadedPhoto{
						File: &tg.InputFile{
							ID:          fileID,
							Parts:       len(chunks),
							Name:        fmt.Sprintf("photo_%d.jpg", photo.ID),
							MD5Checksum: "",
						},
					},
					Message:  messageText,
					RandomID: rand.Int63(),
				})
				if err != nil {
					fmt.Printf("Error sending photo message: %v\n", err)
					return nil
				}
				fmt.Println("Successfully sent photo message")

			case *tg.MessageMediaDocument:
				fmt.Println("Processing document message")
				// Similar logging for document handling...
				// ...
			default:
				fmt.Printf("Unhandled media type: %T, sending as text-only\n", m)
				// For text-only messages
				_, err = c.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
					Peer:     targetPeer,
					Message:  messageText,
					RandomID: rand.Int63(),
				})
				if err != nil {
					fmt.Printf("Error sending text message: %v\n", err)
					return nil
				}
				fmt.Println("Successfully sent text-only message")
			}
		} else {
			fmt.Println("Message contains no media, sending as text-only")
			// For text-only messages
			_, err = c.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
				Peer:     targetPeer,
				Message:  messageText,
				RandomID: rand.Int63(),
			})
			if err != nil {
				fmt.Printf("Error sending text message: %v\n", err)
				return nil
			}
			fmt.Println("Successfully sent text-only message")
		}

		fmt.Printf("Successfully forwarded message from %s to target channel\n", channelTitle)
		return nil
	})

	fmt.Println("Registered message handler")

	// Now authenticate
	if err := c.authenticate(ctx); err != nil {
		return fmt.Errorf("error authenticating: %w", err)
	}
	fmt.Println("Successfully authenticated")

	// Start receiving updates
	fmt.Println("Starting update loop...")

	// Get initial channel states
	fmt.Println("Getting initial channel states...")
	for channelID := range channels {
		fmt.Printf("Getting initial state for channel %d\n", channelID)
		_, err := c.api.UpdatesGetChannelDifference(ctx, &tg.UpdatesGetChannelDifferenceRequest{
			Channel: &tg.InputChannel{
				ChannelID:  channelID,
				AccessHash: 0,
			},
			Filter: &tg.ChannelMessagesFilterEmpty{},
			Pts:    0,
			Limit:  100,
		})
		if err != nil {
			fmt.Printf("Error getting channel difference for %d: %v\n", channelID, err)
		} else {
			fmt.Printf("Successfully got initial state for channel %d\n", channelID)
		}
	}

	fmt.Println("Entering main loop...")
	<-ctx.Done()
	fmt.Println("Update loop terminated")
	return nil
}

func RunClient(ctx context.Context, cfg *config.Config, searchUser *types.User, groups []types.Group, format OutputFormat, exportMetadata bool) error {
	client := NewClient(cfg)
	return client.Run(ctx, searchUser, groups, format, exportMetadata)
}
