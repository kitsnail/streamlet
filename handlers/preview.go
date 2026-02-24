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

// ProgressCallback is called during generation
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
	if err := os.MkdirAll(pg.cfg.ThumbnailDir, 0755); err != nil {
		return err
	}

	// Find all video files from all directories
	var videos []string
	for dirIndex, videoDir := range pg.cfg.VideoDirs {
		filepath.WalkDir(videoDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".mp4") {
				if strings.HasPrefix(d.Name(), "._") {
					return nil
				}
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

	log.Printf("🎬 Found %d videos, generating previews with %d workers...", len(videos), pg.workers)

	jobs := make(chan string, len(videos))
	results := make(chan struct {
		path string
		err  error
	}, len(videos))

	var wg sync.WaitGroup

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

	go func() {
		for _, video := range videos {
			jobs <- video
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

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
		if pg.callback != nil {
			pg.callback(total, success, failed)
		}
	}

	log.Printf("🎬 Preview generation complete: %d success, %d failed", success, failed)
	return nil
}

// generatePreview generates a preview for a single video (60 segments, 0.5 second each = 30 seconds total)
func (pg *PreviewGenerator) generatePreview(prefixedPath string) error {
	hash := md5.Sum([]byte(prefixedPath + "_preview"))
	previewFilename := hex.EncodeToString(hash[:]) + ".mp4"
	previewPath := filepath.Join(pg.cfg.ThumbnailDir, previewFilename)

	if _, err := os.Stat(previewPath); err == nil {
		return nil
	}

	// Parse prefixed path
	absVideoPath, err := parseVideoPath(prefixedPath, pg.cfg)
	if err != nil {
		return err
	}

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
	if duration < 30 {
		duration = 600
	}

	// Generate segments, 0.5 second each, evenly distributed
	// Timestamps: ~2%, 3.6%, 5.2%, ..., 98% of duration (every ~1.6%)
	segments := pg.cfg.PreviewSegments
	const segmentDuration = 0.5
	
	tempDir := filepath.Join(pg.cfg.ThumbnailDir, "temp_"+hex.EncodeToString(hash[:])[:8])
	os.MkdirAll(tempDir, 0755)
	defer os.RemoveAll(tempDir)

	segmentFiles := make([]string, segments)
	success := true

	for i := 0; i < segments; i++ {
		// Distribute timestamps: start from ~2%, end at ~98%
		ts := duration * float64(2+i*96/segments) / 100.0
		segmentPath := filepath.Join(tempDir, fmt.Sprintf("seg%d.ts", i))
		segmentFiles[i] = segmentPath

		cmd := exec.Command("ffmpeg",
			"-y",
			"-ss", fmt.Sprintf("%.2f", ts),
			"-i", absVideoPath,
			"-t", fmt.Sprintf("%.1f", segmentDuration),
			"-c:v", "libx264",
			"-crf", "28",
			"-preset", "fast",
			"-an",
			"-f", "mpegts",
			segmentPath,
		)
		if err := cmd.Run(); err != nil {
			success = false
			break
		}
	}

	if success {
		concatList := "concat:" + strings.Join(segmentFiles, "|")
		concatCmd := exec.Command("ffmpeg",
			"-y",
			"-i", concatList,
			"-c", "copy",
			"-movflags", "+faststart",
			previewPath,
		)
		if err := concatCmd.Run(); err != nil {
			success = false
		}
	}

	if !success {
		// Fallback: simple 30-second preview from middle
		midPoint := duration / 2
		if midPoint < 15 {
			midPoint = 0
		}
		fallbackCmd := exec.Command("ffmpeg",
			"-y",
			"-ss", fmt.Sprintf("%.2f", midPoint),
			"-i", absVideoPath,
			"-t", "30",
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

// GetPreview returns or generates a preview video
func GetPreview(cfg *config.Config) gin.HandlerFunc {
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

		// Security check
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

		hash := md5.Sum([]byte(videoPath + "_preview"))
		previewFilename := hex.EncodeToString(hash[:]) + ".mp4"
		previewPath := filepath.Join(cfg.ThumbnailDir, previewFilename)

		if _, err := os.Stat(previewPath); err == nil {
			c.File(previewPath)
			return
		}

		pg := NewPreviewGenerator(cfg, 1)
		if err := pg.generatePreview(videoPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate preview"})
			return
		}

		c.File(previewPath)
	}
}
