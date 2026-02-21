package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kitsnail/streamlet/config"
	"github.com/kitsnail/streamlet/storage"
)

// PlaylistHandler handles playlist operations
func PlaylistHandler(cfg *config.Config, playlistStore *storage.PlaylistStorage) gin.HandlerFunc {
	return func(c *gin.Context) {
		playlists := playlistStore.GetAll()
		c.JSON(http.StatusOK, gin.H{
			"playlists": playlists,
		})
	}
}

// CreatePlaylistHandler creates a new playlist
func CreatePlaylistHandler(cfg *config.Config, playlistStore *storage.PlaylistStorage) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		if req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Name is required"})
			return
		}

		playlist := playlistStore.Create(req.Name, req.Description)
		c.JSON(http.StatusOK, playlist)
	}
}

// GetPlaylistHandler returns a single playlist
func GetPlaylistHandler(cfg *config.Config, playlistStore *storage.PlaylistStorage) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		playlist := playlistStore.Get(id)
		if playlist == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Playlist not found"})
			return
		}
		c.JSON(http.StatusOK, playlist)
	}
}

// UpdatePlaylistHandler updates a playlist
func UpdatePlaylistHandler(cfg *config.Config, playlistStore *storage.PlaylistStorage) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		playlist := playlistStore.Update(id, req.Name, req.Description)
		if playlist == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Playlist not found"})
			return
		}
		c.JSON(http.StatusOK, playlist)
	}
}

// DeletePlaylistHandler deletes a playlist
func DeletePlaylistHandler(cfg *config.Config, playlistStore *storage.PlaylistStorage) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if !playlistStore.Delete(id) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Playlist not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Playlist deleted"})
	}
}

// AddToPlaylistHandler adds a video to a playlist
func AddToPlaylistHandler(cfg *config.Config, playlistStore *storage.PlaylistStorage) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			PlaylistID string `json:"playlistId"`
			VideoPath  string `json:"videoPath"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		if !playlistStore.AddVideo(req.PlaylistID, req.VideoPath) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Playlist not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Video added to playlist"})
	}
}

// RemoveFromPlaylistHandler removes a video from a playlist
func RemoveFromPlaylistHandler(cfg *config.Config, playlistStore *storage.PlaylistStorage) gin.HandlerFunc {
	return func(c *gin.Context) {
		playlistID := c.Param("id")
		videoPath := c.Query("video")

		if videoPath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Video path is required"})
			return
		}

		if !playlistStore.RemoveVideo(playlistID, videoPath) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Playlist or video not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Video removed from playlist"})
	}
}
