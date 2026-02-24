package config

import (
	"fmt"
	"os"
	"strings"
	"strconv"
)

type Config struct {
	VideoDirs     []string // Multiple video directories
	VideoDir      string   // First video directory (for backward compatibility)
	ThumbnailDir  string
	DataDir       string
	JWTSecret     string
	Username      string
	Password      string
	Env           string
	PreviewSegments  int      // Number of preview segments (default: 60)
}

func Load() *Config {
	videoDirs := parseVideoDirs()
	videoDir := ""
	if len(videoDirs) > 0 {
		videoDir = videoDirs[0]
	}

	return &Config{
		VideoDirs:    videoDirs,
		VideoDir:     videoDir,
		ThumbnailDir: getEnv("THUMBNAIL_DIR", "./thumbnails"),
		DataDir:      getEnv("DATA_DIR", "./data"),
		JWTSecret:    getEnv("JWT_SECRET", "streamlet-secret-change-me"),
		Username:     getEnv("AUTH_USER", "admin"),
		Password:     getEnv("AUTH_PASS", "admin123"),
		Env:             getEnv("ENV", "development"),
		PreviewSegments: getEnvInt("PREVIEW_SEGMENTS", 60),
	}
}

// parseVideoDirs parses video directories from environment variables
// Supports two formats:
// 1. Comma-separated: VIDEO_DIRS=/path1,/path2,/path3
// 2. Indexed: VIDEO_DIR_1=/path1, VIDEO_DIR_2=/path2, VIDEO_DIR_3=/path3
// Falls back to VIDEO_DIR for backward compatibility
func parseVideoDirs() []string {
	var dirs []string

	// First try VIDEO_DIRS (comma-separated)
	if videoDirs := os.Getenv("VIDEO_DIRS"); videoDirs != "" {
		for _, dir := range strings.Split(videoDirs, ",") {
			dir = strings.TrimSpace(dir)
			if dir != "" {
				dirs = append(dirs, dir)
			}
		}
		if len(dirs) > 0 {
			return dirs
		}
	}

	// Then try indexed VIDEO_DIR_1, VIDEO_DIR_2, ...
	for i := 1; i <= 100; i++ {
		dir := os.Getenv(fmt.Sprintf("VIDEO_DIR_%d", i))
		if dir != "" {
			dirs = append(dirs, dir)
		} else if i > 1 {
			break
		}
	}
	if len(dirs) > 0 {
		return dirs
	}

	// Fallback to single VIDEO_DIR
	if dir := os.Getenv("VIDEO_DIR"); dir != "" {
		return []string{dir}
	}

	return []string{"./videos"}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return fallback
}
