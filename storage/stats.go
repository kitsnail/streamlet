package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// VideoStats represents statistics for a video
type VideoStats struct {
	Path        string    `json:"path"`
	Name        string    `json:"name"`
	Views       int       `json:"views"`
	Likes       int       `json:"likes"`
	Liked       bool      `json:"liked"`
	LastViewed  time.Time `json:"lastViewed"`
	Hotness     float64   `json:"hotness"`
}

// Storage handles video statistics persistence
type Storage struct {
	filePath string
	stats    map[string]*VideoStats
	mu       sync.RWMutex
}

// NewStorage creates a new storage instance
func NewStorage(dataDir string) *Storage {
	s := &Storage{
		filePath: filepath.Join(dataDir, "video-stats.json"),
		stats:    make(map[string]*VideoStats),
	}
	s.load()
	return s
}

// load reads stats from file
func (s *Storage) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet
		}
		return err
	}

	var stats map[string]*VideoStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return err
	}

	s.stats = stats
	return nil
}

// save writes stats to file
func (s *Storage) save() error {
	data, err := json.MarshalIndent(s.stats, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// GetStats returns stats for a video
func (s *Storage) GetStats(path string) *VideoStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if stats, ok := s.stats[path]; ok {
		return stats
	}

	return &VideoStats{
		Path:   path,
		Views:  0,
		Likes:  0,
		Liked:  false,
		Hotness: 0,
	}
}

// GetAllStats returns all video stats
func (s *Storage) GetAllStats() map[string]*VideoStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*VideoStats)
	for k, v := range s.stats {
		result[k] = v
	}
	return result
}

// IncrementViews increments view count for a video
func (s *Storage) IncrementViews(path, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats, ok := s.stats[path]
	if !ok {
		stats = &VideoStats{
			Path: path,
			Name: name,
		}
		s.stats[path] = stats
	}

	stats.Views++
	stats.LastViewed = time.Now()
	s.updateHotness(stats)
	s.save()
}

// ToggleLike toggles like status for a video
func (s *Storage) ToggleLike(path, name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats, ok := s.stats[path]
	if !ok {
		stats = &VideoStats{
			Path: path,
			Name: name,
		}
		s.stats[path] = stats
	}

	stats.Liked = !stats.Liked
	if stats.Liked {
		stats.Likes++
	} else {
		stats.Likes--
	}
	s.updateHotness(stats)
	s.save()

	return stats.Liked
}

// updateHotness calculates hotness score
// Hotness = views * 1.0 + likes * 5.0 + recency bonus
func (s *Storage) updateHotness(stats *VideoStats) {
	daysSinceViewed := time.Since(stats.LastViewed).Hours() / 24
	
	// Recency bonus: videos viewed in last 7 days get bonus
	recencyBonus := 0.0
	if daysSinceViewed < 7 {
		recencyBonus = (7 - daysSinceViewed) * 10
	}

	stats.Hotness = float64(stats.Views)*1.0 + float64(stats.Likes)*5.0 + recencyBonus
}
