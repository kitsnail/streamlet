package storage

import (
	"database/sql"
	"time"
)

type VideoStats struct {
	Path          string    `json:"path"`
	Name          string    `json:"name"`
	Views         int       `json:"views"`
	Likes         int       `json:"likes"`
	Liked         bool      `json:"liked"`
	LastViewed    time.Time `json:"lastViewed"`
	Hotness       float64   `json:"hotness"`
	ThumbnailHash string    `json:"thumbnailHash"`
	PreviewHash   string    `json:"previewHash"`
}

type Storage struct {
	db *sql.DB
}

func NewStorage(dataDir string) *Storage {
	db, err := InitDB(dataDir)
	if err != nil {
		panic(err)
	}
	return &Storage{db: db}
}

func (s *Storage) GetStats(path string) *VideoStats {
	var stats VideoStats
	var lastViewed sql.NullTime
	var name sql.NullString

	err := s.db.QueryRow(`
		SELECT path, name, views, likes, liked, last_viewed, hotness
		FROM video_stats WHERE path = ?
	`, path).Scan(&stats.Path, &name, &stats.Views, &stats.Likes, &stats.Liked, &lastViewed, &stats.Hotness)

	if err == sql.ErrNoRows {
		return &VideoStats{
			Path:    path,
			Views:   0,
			Likes:   0,
			Liked:   false,
			Hotness: 0,
		}
	}

	if err != nil {
		return &VideoStats{
			Path:    path,
			Views:   0,
			Likes:   0,
			Liked:   false,
			Hotness: 0,
		}
	}

	stats.Name = name.String
	if lastViewed.Valid {
		stats.LastViewed = lastViewed.Time
	}

	return &stats
}

func (s *Storage) GetAllStats() map[string]*VideoStats {
	rows, err := s.db.Query(`
		SELECT path, name, views, likes, liked, last_viewed, hotness
		FROM video_stats
	`)
	if err != nil {
		return make(map[string]*VideoStats)
	}
	defer rows.Close()

	result := make(map[string]*VideoStats)
	for rows.Next() {
		var stats VideoStats
		var lastViewed sql.NullTime
		var name sql.NullString

		err := rows.Scan(&stats.Path, &name, &stats.Views, &stats.Likes, &stats.Liked, &lastViewed, &stats.Hotness)
		if err != nil {
			continue
		}

		stats.Name = name.String
		if lastViewed.Valid {
			stats.LastViewed = lastViewed.Time
		}
		result[stats.Path] = &stats
	}

	return result
}

func (s *Storage) IncrementViews(path, name string) {
	now := time.Now()
	_, err := s.db.Exec(`
		INSERT INTO video_stats (path, name, views, last_viewed, updated_at)
		VALUES (?, ?, 1, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(path) DO UPDATE SET
			views = views + 1,
			name = COALESCE(NULLIF(?, ''), name),
			last_viewed = ?,
			updated_at = CURRENT_TIMESTAMP
	`, path, name, now, name, now)

	if err != nil {
		return
	}

	s.updateHotness(path)
}

func (s *Storage) ToggleLike(path, name string) bool {
	var liked bool
	err := s.db.QueryRow(`SELECT liked FROM video_stats WHERE path = ?`, path).Scan(&liked)
	if err == sql.ErrNoRows {
		liked = false
	} else if err != nil {
		return false
	}

	newLiked := !liked

	if liked {
		_, err = s.db.Exec(`
			INSERT INTO video_stats (path, name, likes, liked, updated_at)
			VALUES (?, ?, 0, 0, CURRENT_TIMESTAMP)
			ON CONFLICT(path) DO UPDATE SET
				likes = likes - 1,
				liked = 0,
				name = COALESCE(NULLIF(?, ''), name),
				updated_at = CURRENT_TIMESTAMP
		`, path, name, name)
	} else {
		_, err = s.db.Exec(`
			INSERT INTO video_stats (path, name, likes, liked, updated_at)
			VALUES (?, ?, 1, 1, CURRENT_TIMESTAMP)
			ON CONFLICT(path) DO UPDATE SET
				likes = likes + 1,
				liked = 1,
				name = COALESCE(NULLIF(?, ''), name),
				updated_at = CURRENT_TIMESTAMP
		`, path, name, name)
	}

	if err != nil {
		return false
	}

	s.updateHotness(path)
	return newLiked
}

func (s *Storage) updateHotness(path string) {
	var views int
	var likes int
	var lastViewed sql.NullTime

	err := s.db.QueryRow(`
		SELECT views, likes, last_viewed FROM video_stats WHERE path = ?
	`, path).Scan(&views, &likes, &lastViewed)

	if err != nil {
		return
	}

	daysSinceViewed := 0.0
	if lastViewed.Valid {
		daysSinceViewed = time.Since(lastViewed.Time).Hours() / 24
	}

	recencyBonus := 0.0
	if daysSinceViewed < 7 {
		recencyBonus = (7 - daysSinceViewed) * 10
	}

	hotness := float64(views)*1.0 + float64(likes)*5.0 + recencyBonus

	s.db.Exec(`UPDATE video_stats SET hotness = ? WHERE path = ?`, hotness, path)
}

// GetThumbnailHash retrieves the thumbnail hash for a video path
func (s *Storage) GetThumbnailHash(path string) string {
	var hash sql.NullString
	err := s.db.QueryRow(`SELECT thumbnail_hash FROM video_stats WHERE path = ?`, path).Scan(&hash)
	if err != nil {
		return ""
	}
	return hash.String
}

// GetPreviewHash retrieves the preview hash for a video path
func (s *Storage) GetPreviewHash(path string) string {
	var hash sql.NullString
	err := s.db.QueryRow(`SELECT preview_hash FROM video_stats WHERE path = ?`, path).Scan(&hash)
	if err != nil {
		return ""
	}
	return hash.String
}

// SetThumbnailHash updates the thumbnail hash for a video path
func (s *Storage) SetThumbnailHash(path, name, hash string) {
	s.db.Exec(`
		INSERT INTO video_stats (path, name, thumbnail_hash, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(path) DO UPDATE SET
			thumbnail_hash = ?,
			name = COALESCE(NULLIF(?, ''), name),
			updated_at = CURRENT_TIMESTAMP
	`, path, name, hash, hash, name)
}

// SetPreviewHash updates the preview hash for a video path
func (s *Storage) SetPreviewHash(path, name, hash string) {
	s.db.Exec(`
		INSERT INTO video_stats (path, name, preview_hash, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(path) DO UPDATE SET
			preview_hash = ?,
			name = COALESCE(NULLIF(?, ''), name),
			updated_at = CURRENT_TIMESTAMP
	`, path, name, hash, hash, name)
}
