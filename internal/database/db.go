package database

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	db *sql.DB
}

func New(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("error opening database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("error connecting to database: %w", err)
	}

	if err := createTables(db); err != nil {
		return nil, fmt.Errorf("error creating tables: %w", err)
	}

	return &DB{db: db}, nil
}

func createTables(db *sql.DB) error {
	// Messages table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			channel_id INTEGER NOT NULL,
			channel_title TEXT NOT NULL,
			channel_username TEXT,
			message_id INTEGER NOT NULL,
			date DATETIME NOT NULL,
			message TEXT,
			url TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(channel_id, message_id)
		);
	`)
	if err != nil {
		return err
	}

	// User status updates table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS user_status_updates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			username TEXT,
			first_name TEXT,
			last_name TEXT,
			status TEXT NOT NULL,
			status_time DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return err
	}

	// Monitored users table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS monitored_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER UNIQUE NOT NULL,
			username TEXT,
			first_name TEXT,
			last_name TEXT,
			added_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return err
	}

	// Channel metadata table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS channel_metadata (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			channel_id INTEGER UNIQUE NOT NULL,
			title TEXT NOT NULL,
			username TEXT,
			member_count INTEGER,
			is_public BOOLEAN,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return err
	}

	// Message filters table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS message_filters (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			pattern TEXT NOT NULL,
			type TEXT NOT NULL, -- 'keyword', 'regex', 'user', 'channel'
			action TEXT NOT NULL, -- 'forward', 'ignore', 'highlight'
			priority INTEGER DEFAULT 0,
			enabled BOOLEAN DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return err
	}

	// Create indices for better performance
	indices := []string{
		"CREATE INDEX IF NOT EXISTS idx_messages_channel_id ON messages(channel_id);",
		"CREATE INDEX IF NOT EXISTS idx_messages_date ON messages(date);",
		"CREATE INDEX IF NOT EXISTS idx_user_status_user_id ON user_status_updates(user_id);",
		"CREATE INDEX IF NOT EXISTS idx_user_status_time ON user_status_updates(status_time);",
		"CREATE INDEX IF NOT EXISTS idx_filters_type ON message_filters(type);",
		"CREATE INDEX IF NOT EXISTS idx_filters_enabled ON message_filters(enabled);",
	}

	for _, idx := range indices {
		if _, err := db.Exec(idx); err != nil {
			return err
		}
	}

	return nil
}

func (d *DB) SaveMessage(channelID int64, channelTitle, channelUsername string, messageID int, date, message, url string) error {
	_, err := d.db.Exec(`
		INSERT OR IGNORE INTO messages (
			channel_id, channel_title, channel_username, message_id, date, message, url
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, channelID, channelTitle, channelUsername, messageID, date, message, url)
	return err
}

// SaveUserStatusUpdate saves a user status update to the database
func (d *DB) SaveUserStatusUpdate(userID int64, username, firstName, lastName, status, statusTime string) error {
	_, err := d.db.Exec(`
		INSERT INTO user_status_updates (
			user_id, username, first_name, last_name, status, status_time
		) VALUES (?, ?, ?, ?, ?, ?)
	`, userID, username, firstName, lastName, status, statusTime)
	return err
}

// AddMonitoredUser adds a user to the monitored users list
func (d *DB) AddMonitoredUser(userID int64, username, firstName, lastName string) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO monitored_users (
			user_id, username, first_name, last_name
		) VALUES (?, ?, ?, ?)
	`, userID, username, firstName, lastName)
	return err
}

// RemoveMonitoredUser removes a user from the monitored users list
func (d *DB) RemoveMonitoredUser(userID int64) error {
	_, err := d.db.Exec("DELETE FROM monitored_users WHERE user_id = ?", userID)
	return err
}

// GetMonitoredUsers retrieves all monitored users
func (d *DB) GetMonitoredUsers() ([]map[string]interface{}, error) {
	rows, err := d.db.Query("SELECT user_id, username, first_name, last_name FROM monitored_users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []map[string]interface{}
	for rows.Next() {
		var userID int64
		var username, firstName, lastName sql.NullString
		if err := rows.Scan(&userID, &username, &firstName, &lastName); err != nil {
			return nil, err
		}
		user := map[string]interface{}{
			"user_id":    userID,
			"username":   username.String,
			"first_name": firstName.String,
			"last_name":  lastName.String,
		}
		users = append(users, user)
	}
	return users, nil
}

// SaveChannelMetadata saves or updates channel metadata
func (d *DB) SaveChannelMetadata(channelID int64, title, username string, memberCount int, isPublic bool) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO channel_metadata (
			channel_id, title, username, member_count, is_public, updated_at
		) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, channelID, title, username, memberCount, isPublic)
	return err
}

// AddMessageFilter adds a new message filter
func (d *DB) AddMessageFilter(name, pattern, filterType, action string, priority int) error {
	_, err := d.db.Exec(`
		INSERT INTO message_filters (
			name, pattern, type, action, priority
		) VALUES (?, ?, ?, ?, ?)
	`, name, pattern, filterType, action, priority)
	return err
}

// GetActiveFilters retrieves all enabled filters
func (d *DB) GetActiveFilters() ([]MessageFilter, error) {
	rows, err := d.db.Query(`
		SELECT id, name, pattern, type, action, priority 
		FROM message_filters 
		WHERE enabled = 1
		ORDER BY priority DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var filters []MessageFilter
	for rows.Next() {
		var filter MessageFilter
		if err := rows.Scan(&filter.ID, &filter.Name, &filter.Pattern, &filter.Type, &filter.Action, &filter.Priority); err != nil {
			return nil, err
		}
		filters = append(filters, filter)
	}
	return filters, nil
}

// DisableFilter disables a message filter
func (d *DB) DisableFilter(filterID int) error {
	_, err := d.db.Exec("UPDATE message_filters SET enabled = 0 WHERE id = ?", filterID)
	return err
}

// EnableFilter enables a message filter
func (d *DB) EnableFilter(filterID int) error {
	_, err := d.db.Exec("UPDATE message_filters SET enabled = 1 WHERE id = ?", filterID)
	return err
}

// GetUserStatusHistory gets status history for a specific user
func (d *DB) GetUserStatusHistory(userID int64, limit int) ([]map[string]interface{}, error) {
	rows, err := d.db.Query(`
		SELECT username, first_name, last_name, status, status_time 
		FROM user_status_updates 
		WHERE user_id = ?
		ORDER BY status_time DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []map[string]interface{}
	for rows.Next() {
		var username, firstName, lastName, status, statusTime sql.NullString
		if err := rows.Scan(&username, &firstName, &lastName, &status, &statusTime); err != nil {
			return nil, err
		}
		entry := map[string]interface{}{
			"username":    username.String,
			"first_name":  firstName.String,
			"last_name":   lastName.String,
			"status":      status.String,
			"status_time": statusTime.String,
		}
		history = append(history, entry)
	}
	return history, nil
}

// MessageFilter represents a message filter
type MessageFilter struct {
	ID       int
	Name     string
	Pattern  string
	Type     string
	Action   string
	Priority int
}

func (d *DB) Close() error {
	return d.db.Close()
}
