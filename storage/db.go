package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	dbInstance *sql.DB
	dbOnce     sync.Once
	dbMutex    sync.RWMutex
)

func InitDB(dataDir string) (*sql.DB, error) {
	var initErr error
	dbOnce.Do(func() {
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			initErr = fmt.Errorf("failed to create data directory: %w", err)
			return
		}

		dbPath := filepath.Join(dataDir, "streamlet.db")
		dsn := fmt.Sprintf("%s?_journal_mode=WAL&_foreign_keys=on", dbPath)

		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			initErr = fmt.Errorf("failed to open database: %w", err)
			return
		}

		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)

		if err := db.Ping(); err != nil {
			initErr = fmt.Errorf("failed to ping database: %w", err)
			return
		}

		if err := runMigrations(db); err != nil {
			initErr = fmt.Errorf("failed to run migrations: %w", err)
			return
		}

		dbInstance = db
	})

	if initErr != nil {
		return nil, initErr
	}
	return dbInstance, nil
}

func runMigrations(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS video_stats (
			path TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			views INTEGER NOT NULL DEFAULT 0,
			likes INTEGER NOT NULL DEFAULT 0,
			liked INTEGER NOT NULL DEFAULT 0,
			last_viewed DATETIME,
			hotness REAL NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create video_stats table: %w", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS playlists (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create playlists table: %w", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS playlist_videos (
			playlist_id TEXT NOT NULL,
			video_path TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0,
			added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (playlist_id, video_path),
			FOREIGN KEY (playlist_id) REFERENCES playlists(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create playlist_videos table: %w", err)
	}

	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_video_stats_hotness ON video_stats(hotness DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_video_stats_views ON video_stats(views DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_video_stats_likes ON video_stats(likes DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_video_stats_last_viewed ON video_stats(last_viewed DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_playlist_videos_playlist_id ON playlist_videos(playlist_id)`,
		`CREATE INDEX IF NOT EXISTS idx_playlists_updated_at ON playlists(updated_at DESC)`,
	}

	for _, indexSQL := range indexes {
		if _, err := db.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

func GetDB() *sql.DB {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	return dbInstance
}

func CloseDB() error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	if dbInstance != nil {
		return dbInstance.Close()
	}
	return nil
}
