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

// ProgressCallback is called during preview generation
type ProgressCallback func(total, done, failed int)

// PreviewGenerator handles batch preview generation
type PreviewGenerator struct {
	cfg      *config.Config
	workers  int
	callback ProgressCallback
}

// NewPreviewGenerator creates a preview generator
func NewPreviewGenerator(cfg *config.Config, workers int) *PreviewGenerator {
	if workers < 1 {
		workers = 4
	}
	return &PreviewGenerator{cfg: cfg, workers: workers}
}

// SetProgressCallback sets the progress callback
func (pg *PreviewGenerator) SetProgressCallback(cb ProgressCallback) {
	pg.callback = cb
}

// GenerateAll generates previews for all videos concurrently
func (pg *PreviewGenerator) GenerateAll() error {
	// Ensure thumbnail directory exists
	if err := os.MkdirAll(pg.cfg.ThumbnailDir, 0755); err != nil {
		return err
	}

	// Find all video files
	var videos []string
	err := filepath.WalkDir(pg.cfg.VideoDir, func(path string, d os.DirEntry, err error) error {
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
			relPath, _ := filepath.Rel(pg.cfg.VideoDir, path)
			videos = append(videos, relPath)
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.Printf("🎬 Found %d videos, generating previews with %d workers...", len(videos), pg.workers)

	// Worker pool
	jobs := make(chan string, len(videos))
	results := make(chan struct {
		path string
		err  error
	}, len(videos))

	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < pg.workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for videoPath := range jobs {
				err := pg.generatePreview(videoPath)
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
			if success%10 == 0 {
				log.Printf("✅ Progress: %d/%d previews generated", success, total)
			}
		}
		// Call progress callback
		if pg.callback != nil {
			pg.callback(total, success, failed)
		}
	}

	log.Printf("🎬 Preview generation complete: %d success, %d failed", success, failed)
	return nil
}

// generatePreview generates a preview for a single video
func (pg *PreviewGenerator) generatePreview(videoPath string) error {
	// Check if preview already exists
	hash := md5.Sum([]byte(videoPath + "_preview"))
	previewFilename := hex.EncodeToString(hash[:]) + ".mp4"
	previewPath := filepath.Join(pg.cfg.ThumbnailDir, previewFilename)

	if _, err := os.Stat(previewPath); err == nil {
		return nil // Already exists
	}

	absVideoPath := filepath.Join(pg.cfg.VideoDir, videoPath)

	// Get video duration
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
	if duration < 10 {
		duration = 600
	}

	// Build ffmpeg command for 10-segment preview
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
		"-preset", "fast", // Use fast preset for better quality/speed balance
		"-an",
		"-movflags", "+faststart",
		previewPath,
	)

	cmd := exec.Command("ffmpeg", args...)
	if err := cmd.Run(); err != nil {
		// Fallback: simple 10-second preview
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
			"-preset", "fast",
			"-an",
			"-movflags", "+faststart",
			previewPath,
		)
		return fallbackCmd.Run()
	}

	return nil
}

// GetPreview returns or generates a preview video (for API handler)
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

		// Check if preview exists
		if _, err := os.Stat(previewPath); err == nil {
			c.File(previewPath)
			return
		}

		// Generate preview on-demand (fallback)
		pg := NewPreviewGenerator(cfg, 1)
		if err := pg.generatePreview(videoPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate preview"})
			return
		}

		c.File(previewPath)
	}
}

// GeneratePreviewsAsync starts preview generation in background
func GeneratePreviewsAsync(cfg *config.Config) {
	go func() {
		generator := NewPreviewGenerator(cfg, 4)
		if err := generator.GenerateAll(); err != nil {
			log.Printf("❌ Preview generation error: %v", err)
		}
	}()
}
