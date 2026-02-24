package handlers

import (
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
	"github.com/kitsnail/streamlet/storage"
)

// ThumbnailGenerator handles batch thumbnail generation
type ThumbnailGenerator struct {
	cfg      *config.Config
	storage  *storage.Storage
	workers  int
	callback ProgressCallback
}

// NewThumbnailGenerator creates a thumbnail generator
func NewThumbnailGenerator(cfg *config.Config, storage *storage.Storage, workers int) *ThumbnailGenerator {
	if workers < 1 {
		workers = 4
	}
	return &ThumbnailGenerator{cfg: cfg, storage: storage, workers: workers}
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

	// Find all video files from all directories
	var videos []string
	for dirIndex, videoDir := range tg.cfg.VideoDirs {
		filepath.WalkDir(videoDir, func(path string, d os.DirEntry, err error) error {
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
				relPath, _ := filepath.Rel(videoDir, path)
				// Prefix with directory index
				prefixedPath := fmt.Sprintf("%d:%s", dirIndex, relPath)
				videos = append(videos, prefixedPath)
			}
			return nil
		})
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
func (tg *ThumbnailGenerator) generateThumbnail(prefixedPath string) error {
	// Parse prefixed path
	absVideoPath, err := parseVideoPath(prefixedPath, tg.cfg)
	if err != nil {
		return err
	}

	// Get video name from path
	videoName := filepath.Base(absVideoPath)

	// Check database for existing hash
	existingHash := tg.storage.GetThumbnailHash(prefixedPath)
	if existingHash != "" {
		thumbnailPath := filepath.Join(tg.cfg.ThumbnailDir, existingHash+".jpg")
		if _, err := os.Stat(thumbnailPath); err == nil {
			return nil // Already exists with valid hash
		}
	}

	// Calculate file content hash
	contentHash, err := storage.GetFileContentHash(absVideoPath)
	if err != nil {
		return fmt.Errorf("failed to calculate content hash: %w", err)
	}

	thumbnailFilename := contentHash + ".jpg"
	thumbnailPath := filepath.Join(tg.cfg.ThumbnailDir, thumbnailFilename)

	// Check if thumbnail already exists (same content)
	if _, err := os.Stat(thumbnailPath); err == nil {
		// File exists, just update database
		tg.storage.SetThumbnailHash(prefixedPath, videoName, contentHash)
		return nil
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

	if err := cmd.Run(); err != nil {
		return err
	}

	// Update database with new hash
	tg.storage.SetThumbnailHash(prefixedPath, videoName, contentHash)
	return nil
}

// GetThumbnail returns or generates a video thumbnail (for API handler)
func GetThumbnail(cfg *config.Config, store *storage.Storage) gin.HandlerFunc {
	return func(c *gin.Context) {
		videoPath := c.Query("video")
		if videoPath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No video specified"})
			return
		}

		// Parse prefixed path
		absVideoPath, err := parseVideoPath(videoPath, cfg)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid video path"})
			return
		}

		// Security check - ensure path is within one of the video directories
		absVideoPath, err = filepath.Abs(absVideoPath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid video path"})
			return
		}

		allowed := false
		for _, videoDir := range cfg.VideoDirs {
			absVideoDir, _ := filepath.Abs(videoDir)
			if strings.HasPrefix(absVideoPath, absVideoDir) {
				allowed = true
				break
			}
		}

		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}

		if _, err := os.Stat(absVideoPath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Video not found"})
			return
		}

		// Check database for existing hash
		existingHash := store.GetThumbnailHash(videoPath)
		if existingHash != "" {
			thumbnailPath := filepath.Join(cfg.ThumbnailDir, existingHash+".jpg")
			if _, err := os.Stat(thumbnailPath); err == nil {
				c.File(thumbnailPath)
				return
			}
		}

		// Calculate file content hash
		contentHash, err := storage.GetFileContentHash(absVideoPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to calculate content hash"})
			return
		}

		thumbnailFilename := contentHash + ".jpg"
		thumbnailPath := filepath.Join(cfg.ThumbnailDir, thumbnailFilename)

		// Check if thumbnail exists (same content already generated)
		if _, err := os.Stat(thumbnailPath); err == nil {
			// Update database and return
			store.SetThumbnailHash(videoPath, filepath.Base(absVideoPath), contentHash)
			c.File(thumbnailPath)
			return
		}

		// Generate thumbnail on-demand (fallback)
		tg := NewThumbnailGenerator(cfg, store, 1)
		if err := tg.generateThumbnail(videoPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate thumbnail"})
			return
		}

		c.File(thumbnailPath)
	}
}
