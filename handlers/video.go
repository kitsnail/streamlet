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
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Path     string  `json:"path"`
	Modified string  `json:"modified"`
	Views    int     `json:"views"`
	Likes    int     `json:"likes"`
	Liked    bool    `json:"liked"`
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
		sortBy := c.DefaultQuery("sort", "modified") // modified, views, likes, hotness, name

		if page < 1 {
			page = 1
		}
		if pageSize < 1 || pageSize > 100 {
			pageSize = 50
		}

		// Get all stats
		allStats := store.GetAllStats()

		err := filepath.WalkDir(cfg.VideoDir, func(path string, d fs.DirEntry, err error) error {
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

				relPath, _ := filepath.Rel(cfg.VideoDir, path)
				
				// Filter by search query
				if search != "" && !strings.Contains(strings.ToLower(d.Name()), strings.ToLower(search)) {
					return nil
				}

				// Get stats
				stats := allStats[relPath]
				if stats == nil {
					stats = &storage.VideoStats{}
				}

				videos = append(videos, Video{
					Name:     d.Name(),
					Size:     info.Size(),
					Path:     relPath,
					Modified: info.ModTime().Format("2006-01-02 15:04"),
					Views:    stats.Views,
					Likes:    stats.Likes,
					Liked:    stats.Liked,
					Hotness:  stats.Hotness,
				})
			}
			return nil
		})

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan videos"})
			return
		}

		// Sort based on sortBy parameter
		switch sortBy {
		case "views":
			sort.Slice(videos, func(i, j int) bool {
				return videos[i].Views > videos[j].Views
			})
		case "likes":
			sort.Slice(videos, func(i, j int) bool {
				return videos[i].Likes > videos[j].Likes
			})
		case "hotness":
			sort.Slice(videos, func(i, j int) bool {
				return videos[i].Hotness > videos[j].Hotness
			})
		case "name":
			sort.Slice(videos, func(i, j int) bool {
				return videos[i].Name < videos[j].Name
			})
		case "size":
			sort.Slice(videos, func(i, j int) bool {
				return videos[i].Size > videos[j].Size
			})
		default: // "modified"
			sort.Slice(videos, func(i, j int) bool {
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
			"videos":     videos[start:end],
		})
	}
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
			"views": stats.Views,
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
			"liked":  liked,
			"likes":  stats.Likes,
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

		// Construct full path
		fullPath := filepath.Join(cfg.VideoDir, filename)

		// Security check - prevent path traversal
		absPath, err := filepath.Abs(fullPath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}

		absVideoDir, _ := filepath.Abs(cfg.VideoDir)
		if !strings.HasPrefix(absPath, absVideoDir) {
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

		// Parse Range header
		rangeParts := strings.Split(strings.TrimPrefix(rangeHeader, "bytes="), "-")
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

		// Seek to start position
		_, err = file.Seek(start, 0)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Seek failed"})
			return
		}

		contentLength := end - start + 1

		// Set headers for partial content
		c.Header("Content-Type", "video/mp4")
		c.Header("Content-Length", strconv.FormatInt(contentLength, 10))
		c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
		c.Header("Accept-Ranges", "bytes")

		// Stream partial content
		c.DataFromReader(http.StatusPartialContent, contentLength, "video/mp4", file, nil)
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
