package filter

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gnomegl/teleslurp/internal/database"
)

type MessageFilter interface {
	ShouldProcess(message string, channelID int64, userID int64) (bool, string)
}

type FilterManager struct {
	filters []MessageFilter
	db      *database.DB
}

func NewFilterManager(db *database.DB) *FilterManager {
	return &FilterManager{
		db:      db,
		filters: []MessageFilter{},
	}
}

// LoadFilters loads all active filters from the database
func (fm *FilterManager) LoadFilters() error {
	dbFilters, err := fm.db.GetActiveFilters()
	if err != nil {
		return fmt.Errorf("error loading filters: %w", err)
	}

	fm.filters = []MessageFilter{}
	for _, f := range dbFilters {
		switch f.Type {
		case "keyword":
			fm.filters = append(fm.filters, &KeywordFilter{
				Keywords: strings.Split(f.Pattern, ","),
				Action:   f.Action,
			})
		case "regex":
			re, err := regexp.Compile(f.Pattern)
			if err != nil {
				fmt.Printf("Invalid regex pattern %s: %v\n", f.Pattern, err)
				continue
			}
			fm.filters = append(fm.filters, &RegexFilter{
				Pattern: re,
				Action:  f.Action,
			})
		case "user":
			// User filter expects comma-separated user IDs
			fm.filters = append(fm.filters, &UserFilter{
				UserIDs: f.Pattern,
				Action:  f.Action,
			})
		case "channel":
			// Channel filter expects comma-separated channel IDs
			fm.filters = append(fm.filters, &ChannelFilter{
				ChannelIDs: f.Pattern,
				Action:     f.Action,
			})
		case "length":
			fm.filters = append(fm.filters, &LengthFilter{
				MinLength: parseMinLength(f.Pattern),
				Action:    f.Action,
			})
		}
	}

	return nil
}

// ProcessMessage runs all filters on a message and returns whether to process it
func (fm *FilterManager) ProcessMessage(message string, channelID int64, userID int64) (bool, string) {
	for _, filter := range fm.filters {
		shouldProcess, action := filter.ShouldProcess(message, channelID, userID)
		if action == "ignore" && !shouldProcess {
			return false, "ignored"
		}
		if action == "highlight" && shouldProcess {
			return true, "highlight"
		}
	}
	return true, "forward"
}

// KeywordFilter filters messages based on keywords
type KeywordFilter struct {
	Keywords []string
	Action   string
}

func (f *KeywordFilter) ShouldProcess(message string, channelID int64, userID int64) (bool, string) {
	messageLower := strings.ToLower(message)
	for _, keyword := range f.Keywords {
		if strings.Contains(messageLower, strings.ToLower(strings.TrimSpace(keyword))) {
			return true, f.Action
		}
	}
	return false, ""
}

// RegexFilter filters messages based on regex patterns
type RegexFilter struct {
	Pattern *regexp.Regexp
	Action  string
}

func (f *RegexFilter) ShouldProcess(message string, channelID int64, userID int64) (bool, string) {
	if f.Pattern.MatchString(message) {
		return true, f.Action
	}
	return false, ""
}

// UserFilter filters messages based on user ID
type UserFilter struct {
	UserIDs string
	Action  string
}

func (f *UserFilter) ShouldProcess(message string, channelID int64, userID int64) (bool, string) {
	userIDStr := fmt.Sprintf("%d", userID)
	userIDs := strings.Split(f.UserIDs, ",")
	for _, id := range userIDs {
		if strings.TrimSpace(id) == userIDStr {
			return true, f.Action
		}
	}
	return false, ""
}

// ChannelFilter filters messages based on channel ID
type ChannelFilter struct {
	ChannelIDs string
	Action     string
}

func (f *ChannelFilter) ShouldProcess(message string, channelID int64, userID int64) (bool, string) {
	channelIDStr := fmt.Sprintf("%d", channelID)
	channelIDs := strings.Split(f.ChannelIDs, ",")
	for _, id := range channelIDs {
		if strings.TrimSpace(id) == channelIDStr {
			return true, f.Action
		}
	}
	return false, ""
}

// LengthFilter filters messages based on length
type LengthFilter struct {
	MinLength int
	Action    string
}

func (f *LengthFilter) ShouldProcess(message string, channelID int64, userID int64) (bool, string) {
	if len(message) >= f.MinLength {
		return true, f.Action
	}
	return false, ""
}

// MediaFilter filters messages based on media presence
type MediaFilter struct {
	RequireMedia bool
	Action       string
}

func (f *MediaFilter) ShouldProcess(message string, channelID int64, userID int64) (bool, string) {
	// This would need to be integrated with the Telegram message object
	// to check for media presence
	return true, f.Action
}

func parseMinLength(pattern string) int {
	var length int
	fmt.Sscanf(pattern, "%d", &length)
	return length
}

// Helper functions for managing filters

// AddKeywordFilter adds a keyword filter to the database
func AddKeywordFilter(db *database.DB, name string, keywords []string, action string, priority int) error {
	pattern := strings.Join(keywords, ",")
	return db.AddMessageFilter(name, pattern, "keyword", action, priority)
}

// AddRegexFilter adds a regex filter to the database
func AddRegexFilter(db *database.DB, name string, pattern string, action string, priority int) error {
	// Validate regex first
	if _, err := regexp.Compile(pattern); err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}
	return db.AddMessageFilter(name, pattern, "regex", action, priority)
}

// AddUserFilter adds a user filter to the database
func AddUserFilter(db *database.DB, name string, userIDs []int64, action string, priority int) error {
	ids := make([]string, len(userIDs))
	for i, id := range userIDs {
		ids[i] = fmt.Sprintf("%d", id)
	}
	pattern := strings.Join(ids, ",")
	return db.AddMessageFilter(name, pattern, "user", action, priority)
}

// AddChannelFilter adds a channel filter to the database
func AddChannelFilter(db *database.DB, name string, channelIDs []int64, action string, priority int) error {
	ids := make([]string, len(channelIDs))
	for i, id := range channelIDs {
		ids[i] = fmt.Sprintf("%d", id)
	}
	pattern := strings.Join(ids, ",")
	return db.AddMessageFilter(name, pattern, "channel", action, priority)
}

// AddLengthFilter adds a length filter to the database
func AddLengthFilter(db *database.DB, name string, minLength int, action string, priority int) error {
	pattern := fmt.Sprintf("%d", minLength)
	return db.AddMessageFilter(name, pattern, "length", action, priority)
}
