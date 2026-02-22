package handlers

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kitsnail/streamlet/config"
)

// GetPreview generates or returns a preview video with 10 segments
func GetPreview(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		videoPath := c.Query("video")
		if videoPath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No video specified"})
			return
		}

		// Security check
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

		if _, err := os.Stat(absVideoPath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Video not found"})
			return
		}

		// Generate preview filename
		hash := md5.Sum([]byte(videoPath + "_preview"))
		previewFilename := hex.EncodeToString(hash[:]) + ".mp4"
		previewPath := filepath.Join(cfg.ThumbnailDir, previewFilename)

		// Create thumbnail directory if not exists
		if err := os.MkdirAll(cfg.ThumbnailDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory"})
			return
		}

		// Check if preview already exists
		if _, err := os.Stat(previewPath); err == nil {
			c.File(previewPath)
			return
		}

		// Get video duration using ffprobe
		durationCmd := exec.Command("ffprobe",
			"-v", "error",
			"-show_entries", "format=duration",
			"-of", "default=noprint_wrappers=1:nokey=1",
			absVideoPath,
		)
		durationOutput, err := durationCmd.Output()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get video duration"})
			return
		}

		durationStr := strings.TrimSpace(string(durationOutput))
		duration, _ := strconv.ParseFloat(durationStr, 64)
		if duration < 10 {
			duration = 600 // Default to 10 minutes if can't determine
		}

		// Create preview: extract frames at 10 evenly spaced timestamps, 1 second each
		// Use a simpler approach: create a preview from multiple segments
		var inputs []string
		var filterParts []string

		for i := 1; i <= 10; i++ {
			timestamp := duration * float64(i) / 11.0
			if timestamp >= duration-1 {
				timestamp = duration - 2
			}
			if timestamp < 0 {
				timestamp = 0
			}
			inputs = append(inputs,
				"-ss", fmt.Sprintf("%.2f", timestamp),
				"-i", absVideoPath,
				"-t", "1",
			)
			filterParts = append(filterParts, fmt.Sprintf("[%d:v]", i-1))
		}

		filterComplex := strings.Join(filterParts, "") + "concat=n=10:v=1:a=0[out]"

		args := []string{"-y"}
		args = append(args, inputs...)
		args = append(args,
			"-filter_complex", filterComplex,
			"-map", "[out]",
			"-c:v", "libx264",
			"-crf", "28",
			"-preset", "ultrafast",
			"-an",
			"-movflags", "+faststart",
			previewPath,
		)

		cmd := exec.Command("ffmpeg", args...)
		if err := cmd.Run(); err != nil {
			// Fallback: create a simple 10-second preview from middle of video
			midPoint := duration / 2
			if midPoint < 5 {
				midPoint = 0
			}
			fallbackCmd := exec.Command("ffmpeg",
				"-y",
				"-ss", fmt.Sprintf("%.2f", midPoint),
				"-i", absVideoPath,
				"-t", "10",
				"-c:v", "libx264",
				"-crf", "28",
				"-preset", "ultrafast",
				"-an",
				"-movflags", "+faststart",
				previewPath,
			)
			if err := fallbackCmd.Run(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate preview"})
				return
			}
		}

		c.File(previewPath)
	}
}
