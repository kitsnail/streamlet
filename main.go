package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/kitsnail/streamlet/config"
	"github.com/kitsnail/streamlet/handlers"
	"github.com/kitsnail/streamlet/storage"
)

var (
	previewRunning   bool
	thumbnailRunning bool
	genMutex         sync.Mutex
	previewProgress  struct {
		Total   int
		Done    int
		Failed  int
		Running bool
	}
	thumbnailProgress struct {
		Total   int
		Done    int
		Failed  int
		Running bool
	}
)

func main() {
	// Load config
	cfg := config.Load()

	// Initialize storage
	videoStore := storage.NewStorage(cfg.DataDir)
	playlistStore := storage.NewPlaylistStorage(cfg.DataDir)

	// Set gin mode
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create router
	r := gin.Default()

	// Load HTML templates
	r.LoadHTMLGlob("static/*.html")

	// Static files
	r.Static("/static", "./static")

	// Public routes
	r.GET("/", func(c *gin.Context) {
		c.Redirect(302, "/login")
	})
	r.GET("/login", handlers.LoginPage)
	r.POST("/api/login", handlers.Login(cfg))
	
	// Protected routes - Videos
	r.GET("/api/videos", handlers.AuthMiddleware(cfg), handlers.VideoListHandler(cfg, videoStore))
	r.GET("/api/video/*filename", handlers.AuthMiddleware(cfg), handlers.StreamVideo(cfg))
	r.GET("/api/thumbnail", handlers.AuthMiddleware(cfg), handlers.GetThumbnail(cfg, videoStore))
	r.GET("/api/preview", handlers.AuthMiddleware(cfg), handlers.GetPreview(cfg, videoStore))
	r.POST("/api/view", handlers.AuthMiddleware(cfg), handlers.VideoViewHandler(cfg, videoStore))
	r.POST("/api/like", handlers.AuthMiddleware(cfg), handlers.VideoLikeHandler(cfg, videoStore))

	// Protected routes - Media generation
	r.POST("/api/previews/generate", handlers.AuthMiddleware(cfg), func(c *gin.Context) {
		genMutex.Lock()
		defer genMutex.Unlock()

		if previewRunning {
			c.JSON(http.StatusConflict, gin.H{
				"error":    "Preview generation already running",
				"progress": previewProgress,
			})
			return
		}

		previewRunning = true
		previewProgress.Running = true

		go func() {
			generator := handlers.NewPreviewGenerator(cfg, videoStore, 4)
			generator.SetProgressCallback(func(total, done, failed int) {
				genMutex.Lock()
				previewProgress.Total = total
				previewProgress.Done = done
				previewProgress.Failed = failed
				genMutex.Unlock()
			})
			generator.GenerateAll()

			genMutex.Lock()
			previewRunning = false
			previewProgress.Running = false
			genMutex.Unlock()
		}()

		c.JSON(http.StatusOK, gin.H{"message": "Preview generation started"})
	})

	r.GET("/api/previews/status", handlers.AuthMiddleware(cfg), func(c *gin.Context) {
		genMutex.Lock()
		defer genMutex.Unlock()
		c.JSON(http.StatusOK, previewProgress)
	})

	r.POST("/api/thumbnails/generate", handlers.AuthMiddleware(cfg), func(c *gin.Context) {
		genMutex.Lock()
		defer genMutex.Unlock()

		if thumbnailRunning {
			c.JSON(http.StatusConflict, gin.H{
				"error":    "Thumbnail generation already running",
				"progress": thumbnailProgress,
			})
			return
		}

		thumbnailRunning = true
		thumbnailProgress.Running = true

		go func() {
			generator := handlers.NewThumbnailGenerator(cfg, videoStore, 4)
			generator.SetProgressCallback(func(total, done, failed int) {
				genMutex.Lock()
				thumbnailProgress.Total = total
				thumbnailProgress.Done = done
				thumbnailProgress.Failed = failed
				genMutex.Unlock()
			})
			generator.GenerateAll()

			genMutex.Lock()
			thumbnailRunning = false
			thumbnailProgress.Running = false
			genMutex.Unlock()
		}()

		c.JSON(http.StatusOK, gin.H{"message": "Thumbnail generation started"})
	})

	r.GET("/api/thumbnails/status", handlers.AuthMiddleware(cfg), func(c *gin.Context) {
		genMutex.Lock()
		defer genMutex.Unlock()
		c.JSON(http.StatusOK, thumbnailProgress)
	})
	
	// Protected routes - Playlists
	r.GET("/api/playlists", handlers.AuthMiddleware(cfg), handlers.PlaylistHandler(cfg, playlistStore))
	r.POST("/api/playlists", handlers.AuthMiddleware(cfg), handlers.CreatePlaylistHandler(cfg, playlistStore))
	r.GET("/api/playlists/:id", handlers.AuthMiddleware(cfg), handlers.GetPlaylistHandler(cfg, playlistStore))
	r.PUT("/api/playlists/:id", handlers.AuthMiddleware(cfg), handlers.UpdatePlaylistHandler(cfg, playlistStore))
	r.DELETE("/api/playlists/:id", handlers.AuthMiddleware(cfg), handlers.DeletePlaylistHandler(cfg, playlistStore))
	r.POST("/api/playlists/add", handlers.AuthMiddleware(cfg), handlers.AddToPlaylistHandler(cfg, playlistStore))
	r.DELETE("/api/playlists/:id/video", handlers.AuthMiddleware(cfg), handlers.RemoveFromPlaylistHandler(cfg, playlistStore))
	
	// Pages
	r.GET("/player", handlers.AuthMiddleware(cfg), handlers.PlayerPage)
	r.GET("/playlists", handlers.AuthMiddleware(cfg), func(c *gin.Context) {
		c.HTML(200, "playlists.html", nil)
	})
	r.GET("/playlist.html", handlers.AuthMiddleware(cfg), func(c *gin.Context) {
		c.HTML(200, "playlist.html", nil)
	})

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🎬 Streamlet running on :%s", port)
	log.Printf("📁 Video directories: %s", strings.Join(cfg.VideoDirs, ", "))
	log.Printf("📊 Data directory: %s", cfg.DataDir)
	
	// Start thumbnail and preview generation in background on startup
	go func() {
		// Run thumbnail generation first (faster)
		tg := handlers.NewThumbnailGenerator(cfg, videoStore, 4)
		if err := tg.GenerateAll(); err != nil {
			log.Printf("❌ Thumbnail generation error: %v", err)
		}
	}()

	go func() {
		// Run preview generation in parallel
		pg := handlers.NewPreviewGenerator(cfg, videoStore, 4)
		if err := pg.GenerateAll(); err != nil {
			log.Printf("❌ Preview generation error: %v", err)
		}
	}()

	log.Fatal(r.Run(":" + port))
}
