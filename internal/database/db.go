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
	return err
}

func (d *DB) SaveMessage(channelID int64, channelTitle, channelUsername string, messageID int, date, message, url string) error {
	_, err := d.db.Exec(`
		INSERT OR IGNORE INTO messages (
			channel_id, channel_title, channel_username, message_id, date, message, url
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, channelID, channelTitle, channelUsername, messageID, date, message, url)
	return err
}

func (d *DB) Close() error {
	return d.db.Close()
}
