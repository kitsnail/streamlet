package handlers

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kitsnail/streamlet/config"
	"github.com/kitsnail/streamlet/storage"
)

// Video represents a video file info
type Video struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Duration   string `json:"duration"`    // Video duration in human readable format
	DurationSec int    `json:"durationSec"` // Video duration in seconds (for filtering)
	Path       string `json:"path"`
	Dir        string `json:"dir,omitempty"` // Source directory index or name
	Modified   string `json:"modified"`
	Views      int    `json:"views"`
	Likes      int    `json:"likes"`
	Liked      bool   `json:"liked"`
	Hotness    float64 `json:"hotness"`
}

// VideoListHandler creates a video list handler with storage
func VideoListHandler(cfg *config.Config, store *storage.Storage) gin.HandlerFunc {
	return func(c *gin.Context) {
		var videos []Video

		// Get query parameters
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "50"))
		search := c.Query("search")
		sortBy := c.DefaultQuery("sort", "modified") // modified, views, likes, hotness, name, size, duration
		order := c.DefaultQuery("order", "desc")     // asc, desc
		durationMin, _ := strconv.Atoi(c.DefaultQuery("durationMin", "0")) // minutes
		durationMax, _ := strconv.Atoi(c.DefaultQuery("durationMax", "0")) // minutes, 0 means no limit

		if page < 1 {
			page = 1
		}
		if pageSize < 1 || pageSize > 100 {
			pageSize = 50
		}

		// Get all stats
		allStats := store.GetAllStats()

		// Scan all video directories
		for dirIndex, videoDir := range cfg.VideoDirs {
			filepath.WalkDir(videoDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				// Skip macOS hidden files (._*.mp4) and small files
				if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".mp4") {
					// Skip macOS AppleDouble files (._filename)
					if strings.HasPrefix(d.Name(), "._") {
						return nil
					}

					info, err := d.Info()
					if err != nil {
						return nil
					}

					// Skip very small files (< 10MB, likely corrupted or placeholder)
					if info.Size() < 10*1024*1024 {
						return nil
					}

					relPath, _ := filepath.Rel(videoDir, path)
					
					// Prefix path with directory index to distinguish sources
					// Format: dirIndex:relPath (e.g., "0:video.mp4", "1:subdir/video.mp4")
					prefixedPath := fmt.Sprintf("%d:%s", dirIndex, relPath)
					
					// Filter by search query
					if search != "" && !strings.Contains(strings.ToLower(d.Name()), strings.ToLower(search)) {
						return nil
					}

					// Get stats
					stats := allStats[prefixedPath]
					if stats == nil {
						stats = &storage.VideoStats{}
					}

					// Get video duration
					duration := ""
					durationSec := 0
					if dur, err := GetMP4Duration(path); err == nil && dur > 0 {
						duration = FormatDuration(dur)
						durationSec = int(dur.Seconds())
					}

					videos = append(videos, Video{
						Name:       d.Name(),
						Size:       info.Size(),
						Duration:   duration,
						DurationSec: durationSec,
						Path:       prefixedPath,
						Dir:        filepath.Base(videoDir),
						Modified:   info.ModTime().Format("2006-01-02 15:04"),
						Views:      stats.Views,
						Likes:      stats.Likes,
						Liked:      stats.Liked,
						Hotness:    stats.Hotness,
					})
				}
				return nil
			})
		}

		// Filter by duration (durationMin and durationMax are in minutes)
		if durationMin > 0 || durationMax > 0 {
			filtered := make([]Video, 0)
			minSec := durationMin * 60
			maxSec := durationMax * 60
			
			for _, v := range videos {
				// If max is 0, only check min
				if durationMax == 0 {
					if v.DurationSec >= minSec {
						filtered = append(filtered, v)
					}
				} else {
					// Check both min and max
					if v.DurationSec >= minSec && v.DurationSec < maxSec {
						filtered = append(filtered, v)
					}
				}
			}
			videos = filtered
		}

		// Sort based on sortBy parameter
		isAsc := order == "asc"
		switch sortBy {
		case "views":
			sort.Slice(videos, func(i, j int) bool {
				if isAsc {
					return videos[i].Views < videos[j].Views
				}
				return videos[i].Views > videos[j].Views
			})
		case "likes":
			sort.Slice(videos, func(i, j int) bool {
				if isAsc {
					return videos[i].Likes < videos[j].Likes
				}
				return videos[i].Likes > videos[j].Likes
			})
		case "hotness":
			sort.Slice(videos, func(i, j int) bool {
				if isAsc {
					return videos[i].Hotness < videos[j].Hotness
				}
				return videos[i].Hotness > videos[j].Hotness
			})
		case "name":
			sort.Slice(videos, func(i, j int) bool {
				if isAsc {
					return videos[i].Name > videos[j].Name
				}
				return videos[i].Name < videos[j].Name
			})
		case "size":
			sort.Slice(videos, func(i, j int) bool {
				if isAsc {
					return videos[i].Size < videos[j].Size
				}
				return videos[i].Size > videos[j].Size
			})
		case "duration":
			sort.Slice(videos, func(i, j int) bool {
				if isAsc {
					return videos[i].DurationSec < videos[j].DurationSec
				}
				return videos[i].DurationSec > videos[j].DurationSec
			})
		
		default: // "modified"
			sort.Slice(videos, func(i, j int) bool {
				if isAsc {
					return videos[i].Modified < videos[j].Modified
				}
				return videos[i].Modified > videos[j].Modified
			})
		}

		// Pagination
		total := len(videos)
		totalPages := (total + pageSize - 1) / pageSize
		start := (page - 1) * pageSize
		end := start + pageSize
		
		if start > total {
			start = total
		}
		if end > total {
			end = total
		}

		c.JSON(http.StatusOK, gin.H{
			"total":      total,
			"page":       page,
			"pageSize":   pageSize,
			"totalPages": totalPages,
			"sort":       sortBy,
			"order":      order,
			"videos":     videos[start:end],
			"videoDirs":  cfg.VideoDirs,
		})
	}
}

// parseVideoPath parses prefixed video path (format: dirIndex:relPath)
// Returns the absolute path to the video file
func parseVideoPath(prefixedPath string, cfg *config.Config) (string, error) {
	// Check if path has prefix
	if strings.Contains(prefixedPath, ":") {
		parts := strings.SplitN(prefixedPath, ":", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid path format")
		}
		
		dirIndex, err := strconv.Atoi(parts[0])
		if err != nil {
			return "", fmt.Errorf("invalid directory index")
		}
		
		if dirIndex < 0 || dirIndex >= len(cfg.VideoDirs) {
			return "", fmt.Errorf("directory index out of range")
		}
		
		return filepath.Join(cfg.VideoDirs[dirIndex], parts[1]), nil
	}
	
	// Fallback: use first directory for backward compatibility
	return filepath.Join(cfg.VideoDir, prefixedPath), nil
}

// VideoViewHandler increments view count
func VideoViewHandler(cfg *config.Config, store *storage.Storage) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Path string `json:"path"`
			Name string `json:"name"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		store.IncrementViews(req.Path, req.Name)
		stats := store.GetStats(req.Path)

		c.JSON(http.StatusOK, gin.H{
			"views":   stats.Views,
			"hotness": stats.Hotness,
		})
	}
}

// VideoLikeHandler toggles like status
func VideoLikeHandler(cfg *config.Config, store *storage.Storage) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Path string `json:"path"`
			Name string `json:"name"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		liked := store.ToggleLike(req.Path, req.Name)
		stats := store.GetStats(req.Path)

		c.JSON(http.StatusOK, gin.H{
			"liked":   liked,
			"likes":   stats.Likes,
			"hotness": stats.Hotness,
		})
	}
}

// StreamVideo streams video file with Range support
func StreamVideo(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get filename from path
		filename := c.Param("filename")
		filename = strings.TrimPrefix(filename, "/")

		// Parse prefixed path
		absPath, err := parseVideoPath(filename, cfg)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		// Security check - ensure path is within one of the video directories
		absPath, err = filepath.Abs(absPath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		// Check if path is within any allowed directory
		allowed := false
		for _, videoDir := range cfg.VideoDirs {
			absVideoDir, _ := filepath.Abs(videoDir)
			if strings.HasPrefix(absPath, absVideoDir) {
				allowed = true
				break
			}
		}

		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}

		// Open file
		file, err := os.Open(absPath)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Video not found"})
			return
		}
		defer file.Close()

		// Get file info
		stat, err := file.Stat()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get file info"})
			return
		}

		// Use http.ServeContent to handle Range requests properly
		// This is the standard way to serve static files with Range support
		c.Header("Content-Type", "video/mp4")
		c.Header("Accept-Ranges", "bytes")
		c.Header("Cache-Control", "public, max-age=31536000") // Cache for 1 year
		http.ServeContent(c.Writer, c.Request, "video.mp4", stat.ModTime(), file)
	}
}

// isBrokenPipe checks if error is a broken pipe or connection reset
func isBrokenPipe(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "connection closed")
}

// PlayerPage renders video player page
func PlayerPage(c *gin.Context) {
	video := c.Query("v")
	if video == "" {
		// 没有视频参数时，显示视频列表页面
		c.HTML(http.StatusOK, "index.html", nil)
		return
	}
	c.HTML(http.StatusOK, "player.html", gin.H{
		"video": video,
	})
}
