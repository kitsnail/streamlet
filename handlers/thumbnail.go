package handlers

import (
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kitsnail/streamlet/config"
)

// GetThumbnail returns or generates video thumbnail
func GetThumbnail(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get video path
		videoPath := c.Query("video")
		if videoPath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No video specified"})
			return
		}

		// Security check - prevent path traversal
		fullVideoPath := filepath.Join(cfg.VideoDir, videoPath)
		absVideoPath, err := filepath.Abs(fullVideoPath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid video path"})
			return
		}

		absVideoDir, _ := filepath.Abs(cfg.VideoDir)
		if !strings.HasPrefix(absVideoPath, absVideoDir) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}

		// Check if video exists
		if _, err := os.Stat(absVideoPath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Video not found"})
			return
		}

		// Generate thumbnail filename (MD5 hash of video path)
		hash := md5.Sum([]byte(videoPath))
		thumbnailFilename := hex.EncodeToString(hash[:]) + ".jpg"
		thumbnailPath := filepath.Join(cfg.ThumbnailDir, thumbnailFilename)

		// Create thumbnail directory if not exists
		if err := os.MkdirAll(cfg.ThumbnailDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create thumbnail directory"})
			return
		}

		// Check if thumbnail already exists
		if _, err := os.Stat(thumbnailPath); err == nil {
			// Thumbnail exists, serve it
			c.File(thumbnailPath)
			return
		}

		// Generate thumbnail using ffmpeg
		// ffmpeg -i video.mp4 -ss 00:00:01 -vframes 1 -q:v 2 thumbnail.jpg
		cmd := exec.Command("ffmpeg",
			"-i", absVideoPath,
			"-ss", "00:00:01",  // Take screenshot at 1 second
			"-vframes", "1",     // Extract one frame
			"-q:v", "2",         // High quality
			"-y",                // Overwrite output file
			thumbnailPath,
		)

		// Run ffmpeg
		if err := cmd.Run(); err != nil {
			// If ffmpeg fails, return a placeholder or error
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate thumbnail"})
			return
		}

		// Serve the generated thumbnail
		c.File(thumbnailPath)
	}
}
