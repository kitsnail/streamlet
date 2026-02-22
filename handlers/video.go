package handlers

import (
	"fmt"
	"io"
	"io/fs"
	"log"
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
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Path     string `json:"path"`
	Dir      string `json:"dir,omitempty"` // Source directory index or name
	Modified string `json:"modified"`
	Views    int    `json:"views"`
	Likes    int    `json:"likes"`
	Liked    bool   `json:"liked"`
	Hotness  float64 `json:"hotness"`
}

// VideoListHandler creates a video list handler with storage
func VideoListHandler(cfg *config.Config, store *storage.Storage) gin.HandlerFunc {
	return func(c *gin.Context) {
		var videos []Video

		// Get query parameters
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "50"))
		search := c.Query("search")
		sortBy := c.DefaultQuery("sort", "modified") // modified, views, likes, hotness, name, size
		order := c.DefaultQuery("order", "desc")     // asc, desc

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

					videos = append(videos, Video{
						Name:     d.Name(),
						Size:     info.Size(),
						Path:     prefixedPath,
						Dir:      filepath.Base(videoDir),
						Modified: info.ModTime().Format("2006-01-02 15:04"),
						Views:    stats.Views,
						Likes:    stats.Likes,
						Liked:    stats.Liked,
						Hotness:  stats.Hotness,
					})
				}
				return nil
			})
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

		fileSize := stat.Size()

		// Handle Range request for video seeking
		rangeHeader := c.GetHeader("Range")
		if rangeHeader == "" {
			// No range header, send entire file
			c.Header("Content-Type", "video/mp4")
			c.Header("Content-Length", strconv.FormatInt(fileSize, 10))
			c.Header("Accept-Ranges", "bytes")
			c.DataFromReader(http.StatusOK, fileSize, "video/mp4", file, nil)
			return
		}

		// Parse Range header (e.g., "bytes=0-1023" or "bytes=0-")
		rangeStr := strings.TrimPrefix(rangeHeader, "bytes=")
		rangeParts := strings.Split(rangeStr, "-")
		if len(rangeParts) != 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid range"})
			return
		}

		start, _ := strconv.ParseInt(rangeParts[0], 10, 64)
		end := fileSize - 1
		if rangeParts[1] != "" {
			end, _ = strconv.ParseInt(rangeParts[1], 10, 64)
		}

		// Validate range
		if start >= fileSize || end >= fileSize || start > end {
			c.Status(http.StatusRequestedRangeNotSatisfiable)
			c.Header("Content-Range", fmt.Sprintf("bytes */%d", fileSize))
			return
		}

		contentLength := end - start + 1

		// Seek to start position
		_, err = file.Seek(start, 0)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Seek failed"})
			return
		}

		// Set headers for partial content
		c.Header("Content-Type", "video/mp4")
		c.Header("Content-Length", strconv.FormatInt(contentLength, 10))
		c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
		c.Header("Accept-Ranges", "bytes")
		c.Status(http.StatusPartialContent)

		// Use io.CopyN to ensure exact byte count
		_, err = io.CopyN(c.Writer, file, contentLength)
		if err != nil && err != io.EOF {
			log.Printf("Stream error: %v", err)
		}
	}
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
