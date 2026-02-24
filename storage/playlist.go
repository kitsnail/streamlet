package storage

import (
	"database/sql"
	"time"
)

type Playlist struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Videos      []string  `json:"videos"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type PlaylistStorage struct {
	db *sql.DB
}

func NewPlaylistStorage(dataDir string) *PlaylistStorage {
	db, err := InitDB(dataDir)
	if err != nil {
		panic(err)
	}
	return &PlaylistStorage{db: db}
}

func generateID() string {
	return time.Now().Format("20060102150405")
}

func (s *PlaylistStorage) Create(name, description string) *Playlist {
	id := generateID()
	now := time.Now()

	_, err := s.db.Exec(`
		INSERT INTO playlists (id, name, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, name, description, now, now)

	if err != nil {
		return nil
	}

	return &Playlist{
		ID:          id,
		Name:        name,
		Description: description,
		Videos:      []string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func (s *PlaylistStorage) Get(id string) *Playlist {
	var playlist Playlist
	var createdAt, updatedAt sql.NullTime

	err := s.db.QueryRow(`
		SELECT id, name, description, created_at, updated_at
		FROM playlists WHERE id = ?
	`, id).Scan(&playlist.ID, &playlist.Name, &playlist.Description, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return nil
	}

	if createdAt.Valid {
		playlist.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		playlist.UpdatedAt = updatedAt.Time
	}

	playlist.Videos = s.getVideos(id)
	return &playlist
}

func (s *PlaylistStorage) getVideos(playlistID string) []string {
	rows, err := s.db.Query(`
		SELECT video_path FROM playlist_videos
		WHERE playlist_id = ? ORDER BY position
	`, playlistID)
	if err != nil {
		return []string{}
	}
	defer rows.Close()

	var videos []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			continue
		}
		videos = append(videos, path)
	}

	if videos == nil {
		videos = []string{}
	}
	return videos
}

func (s *PlaylistStorage) GetAll() []*Playlist {
	rows, err := s.db.Query(`
		SELECT id, name, description, created_at, updated_at
		FROM playlists ORDER BY updated_at DESC
	`)
	if err != nil {
		return []*Playlist{}
	}
	defer rows.Close()

	var playlists []*Playlist
	for rows.Next() {
		var p Playlist
		var createdAt, updatedAt sql.NullTime

		err := rows.Scan(&p.ID, &p.Name, &p.Description, &createdAt, &updatedAt)
		if err != nil {
			continue
		}

		if createdAt.Valid {
			p.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			p.UpdatedAt = updatedAt.Time
		}

		p.Videos = s.getVideos(p.ID)
		playlists = append(playlists, &p)
	}

	if playlists == nil {
		playlists = []*Playlist{}
	}
	return playlists
}

func (s *PlaylistStorage) Update(id, name, description string) *Playlist {
	now := time.Now()

	if name != "" && description != "" {
		_, err := s.db.Exec(`
			UPDATE playlists SET name = ?, description = ?, updated_at = ? WHERE id = ?
		`, name, description, now, id)
		if err != nil {
			return nil
		}
	} else if name != "" {
		_, err := s.db.Exec(`
			UPDATE playlists SET name = ?, updated_at = ? WHERE id = ?
		`, name, now, id)
		if err != nil {
			return nil
		}
	} else if description != "" {
		_, err := s.db.Exec(`
			UPDATE playlists SET description = ?, updated_at = ? WHERE id = ?
		`, description, now, id)
		if err != nil {
			return nil
		}
	}

	return s.Get(id)
}

func (s *PlaylistStorage) Delete(id string) bool {
	_, err := s.db.Exec(`DELETE FROM playlists WHERE id = ?`, id)
	return err == nil
}

func (s *PlaylistStorage) AddVideo(playlistID, videoPath string) bool {
	var maxPos int
	err := s.db.QueryRow(`
		SELECT COALESCE(MAX(position), -1) FROM playlist_videos WHERE playlist_id = ?
	`, playlistID).Scan(&maxPos)
	if err != nil {
		return false
	}

	_, err = s.db.Exec(`
		INSERT OR IGNORE INTO playlist_videos (playlist_id, video_path, position, added_at)
		VALUES (?, ?, ?, ?)
	`, playlistID, videoPath, maxPos+1, time.Now())

	if err != nil {
		return false
	}

	s.db.Exec(`UPDATE playlists SET updated_at = ? WHERE id = ?`, time.Now(), playlistID)
	return true
}

func (s *PlaylistStorage) RemoveVideo(playlistID, videoPath string) bool {
	_, err := s.db.Exec(`
		DELETE FROM playlist_videos WHERE playlist_id = ? AND video_path = ?
	`, playlistID, videoPath)

	if err != nil {
		return false
	}

	s.db.Exec(`UPDATE playlists SET updated_at = ? WHERE id = ?`, time.Now(), playlistID)
	return true
}

func (s *PlaylistStorage) ReorderVideos(playlistID string, videoPaths []string) bool {
	tx, err := s.db.Begin()
	if err != nil {
		return false
	}
	defer tx.Rollback()

	_, err = tx.Exec(`DELETE FROM playlist_videos WHERE playlist_id = ?`, playlistID)
	if err != nil {
		return false
	}

	now := time.Now()
	for i, path := range videoPaths {
		_, err = tx.Exec(`
			INSERT INTO playlist_videos (playlist_id, video_path, position, added_at)
			VALUES (?, ?, ?, ?)
		`, playlistID, path, i, now)
		if err != nil {
			return false
		}
	}

	_, err = tx.Exec(`UPDATE playlists SET updated_at = ? WHERE id = ?`, now, playlistID)
	if err != nil {
		return false
	}

	return tx.Commit() == nil
}
