package handlers

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/kitsnail/streamlet/config"
)

// ThumbnailGenerator handles batch thumbnail generation
type ThumbnailGenerator struct {
	cfg      *config.Config
	workers  int
	callback ProgressCallback
}

// NewThumbnailGenerator creates a thumbnail generator
func NewThumbnailGenerator(cfg *config.Config, workers int) *ThumbnailGenerator {
	if workers < 1 {
		workers = 4
	}
	return &ThumbnailGenerator{cfg: cfg, workers: workers}
}

// SetProgressCallback sets the progress callback
func (tg *ThumbnailGenerator) SetProgressCallback(cb ProgressCallback) {
	tg.callback = cb
}

// GenerateAll generates thumbnails for all videos concurrently
func (tg *ThumbnailGenerator) GenerateAll() error {
	// Ensure thumbnail directory exists
	if err := os.MkdirAll(tg.cfg.ThumbnailDir, 0755); err != nil {
		return err
	}

	// Find all video files
	var videos []string
	err := filepath.WalkDir(tg.cfg.VideoDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".mp4") {
			// Skip macOS AppleDouble files
			if strings.HasPrefix(d.Name(), "._") {
				return nil
			}
			// Skip small files
			info, err := d.Info()
			if err != nil {
				return nil
			}
			if info.Size() < 10*1024*1024 {
				return nil
			}
			relPath, _ := filepath.Rel(tg.cfg.VideoDir, path)
			videos = append(videos, relPath)
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.Printf("🖼️  Found %d videos, generating thumbnails with %d workers...", len(videos), tg.workers)

	// Worker pool
	jobs := make(chan string, len(videos))
	results := make(chan struct {
		path string
		err  error
	}, len(videos))

	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < tg.workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for videoPath := range jobs {
				err := tg.generateThumbnail(videoPath)
				results <- struct {
					path string
					err  error
				}{path: videoPath, err: err}
			}
		}(i)
	}

	// Send jobs
	go func() {
		for _, video := range videos {
			jobs <- video
		}
		close(jobs)
	}()

	// Wait for completion
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	success := 0
	failed := 0
	total := len(videos)

	for result := range results {
		if result.err != nil {
			log.Printf("❌ Failed: %s - %v", result.path, result.err)
			failed++
		} else {
			success++
			if success%20 == 0 {
				log.Printf("✅ Progress: %d/%d thumbnails generated", success, total)
			}
		}
		// Call progress callback
		if tg.callback != nil {
			tg.callback(total, success, failed)
		}
	}

	log.Printf("🖼️  Thumbnail generation complete: %d success, %d failed", success, failed)
	return nil
}

// generateThumbnail generates a thumbnail for a single video
func (tg *ThumbnailGenerator) generateThumbnail(videoPath string) error {
	// Check if thumbnail already exists
	hash := md5.Sum([]byte(videoPath))
	thumbnailFilename := hex.EncodeToString(hash[:]) + ".jpg"
	thumbnailPath := filepath.Join(tg.cfg.ThumbnailDir, thumbnailFilename)

	if _, err := os.Stat(thumbnailPath); err == nil {
		return nil // Already exists
	}

	absVideoPath := filepath.Join(tg.cfg.VideoDir, videoPath)

	// Get video duration using ffprobe
	durationCmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		absVideoPath,
	)
	durationOutput, err := durationCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get duration: %w", err)
	}

	durationStr := strings.TrimSpace(string(durationOutput))
	duration, _ := strconv.ParseFloat(durationStr, 64)
	if duration < 1 {
		duration = 600 // Default to 10 minutes
	}

	// Take screenshot at middle of video
	middlePoint := duration / 2

	// Generate thumbnail using ffmpeg
	cmd := exec.Command("ffmpeg",
		"-i", absVideoPath,
		"-ss", fmt.Sprintf("%.2f", middlePoint), // Take screenshot at middle
		"-vframes", "1",                         // Extract one frame
		"-q:v", "2",                             // High quality
		"-y",                                    // Overwrite output file
		thumbnailPath,
	)

	return cmd.Run()
}

// GetThumbnail returns or generates a video thumbnail (for API handler)
func GetThumbnail(cfg *config.Config) gin.HandlerFunc {
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

		// Generate thumbnail filename
		hash := md5.Sum([]byte(videoPath))
		thumbnailFilename := hex.EncodeToString(hash[:]) + ".jpg"
		thumbnailPath := filepath.Join(cfg.ThumbnailDir, thumbnailFilename)

		// Check if thumbnail exists
		if _, err := os.Stat(thumbnailPath); err == nil {
			c.File(thumbnailPath)
			return
		}

		// Generate thumbnail on-demand (fallback)
		tg := NewThumbnailGenerator(cfg, 1)
		if err := tg.generateThumbnail(videoPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate thumbnail"})
			return
		}

		c.File(thumbnailPath)
	}
}
