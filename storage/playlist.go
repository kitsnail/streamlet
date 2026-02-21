package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Playlist represents a video playlist
type Playlist struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Videos      []string  `json:"videos"` // Video paths
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// PlaylistStorage handles playlist persistence
type PlaylistStorage struct {
	filePath   string
	playlists  map[string]*Playlist
	mu         sync.RWMutex
}

// NewPlaylistStorage creates a new playlist storage instance
func NewPlaylistStorage(dataDir string) *PlaylistStorage {
	s := &PlaylistStorage{
		filePath:  filepath.Join(dataDir, "playlists.json"),
		playlists: make(map[string]*Playlist),
	}
	s.load()
	return s
}

// load reads playlists from file
func (s *PlaylistStorage) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var playlists map[string]*Playlist
	if err := json.Unmarshal(data, &playlists); err != nil {
		return err
	}

	s.playlists = playlists
	return nil
}

// save writes playlists to file
func (s *PlaylistStorage) save() error {
	data, err := json.MarshalIndent(s.playlists, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// generateID generates a unique playlist ID
func generateID() string {
	return time.Now().Format("20060102150405")
}

// Create creates a new playlist
func (s *PlaylistStorage) Create(name, description string) *Playlist {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := generateID()
	playlist := &Playlist{
		ID:          id,
		Name:        name,
		Description: description,
		Videos:      []string{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	s.playlists[id] = playlist
	s.save()

	return playlist
}

// Get returns a playlist by ID
func (s *PlaylistStorage) Get(id string) *Playlist {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.playlists[id]
}

// GetAll returns all playlists
func (s *PlaylistStorage) GetAll() []*Playlist {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Playlist, 0, len(s.playlists))
	for _, p := range s.playlists {
		result = append(result, p)
	}
	return result
}

// Update updates a playlist
func (s *PlaylistStorage) Update(id, name, description string) *Playlist {
	s.mu.Lock()
	defer s.mu.Unlock()

	playlist, ok := s.playlists[id]
	if !ok {
		return nil
	}

	if name != "" {
		playlist.Name = name
	}
	if description != "" {
		playlist.Description = description
	}
	playlist.UpdatedAt = time.Now()
	s.save()

	return playlist
}

// Delete deletes a playlist
func (s *PlaylistStorage) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.playlists[id]; !ok {
		return false
	}

	delete(s.playlists, id)
	s.save()
	return true
}

// AddVideo adds a video to a playlist
func (s *PlaylistStorage) AddVideo(playlistID, videoPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	playlist, ok := s.playlists[playlistID]
	if !ok {
		return false
	}

	// Check if video already in playlist
	for _, v := range playlist.Videos {
		if v == videoPath {
			return true // Already exists
		}
	}

	playlist.Videos = append(playlist.Videos, videoPath)
	playlist.UpdatedAt = time.Now()
	s.save()

	return true
}

// RemoveVideo removes a video from a playlist
func (s *PlaylistStorage) RemoveVideo(playlistID, videoPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	playlist, ok := s.playlists[playlistID]
	if !ok {
		return false
	}

	for i, v := range playlist.Videos {
		if v == videoPath {
			playlist.Videos = append(playlist.Videos[:i], playlist.Videos[i+1:]...)
			playlist.UpdatedAt = time.Now()
			s.save()
			return true
		}
	}

	return false
}

// ReorderVideos reorders videos in a playlist
func (s *PlaylistStorage) ReorderVideos(playlistID string, videoPaths []string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	playlist, ok := s.playlists[playlistID]
	if !ok {
		return false
	}

	playlist.Videos = videoPaths
	playlist.UpdatedAt = time.Now()
	s.save()

	return true
}
