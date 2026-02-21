package config

import (
	"os"
)

type Config struct {
	VideoDir      string
	ThumbnailDir  string
	DataDir       string
	JWTSecret     string
	Username      string
	Password      string
	Env           string
}

func Load() *Config {
	return &Config{
		VideoDir:     getEnv("VIDEO_DIR", "./videos"),
		ThumbnailDir: getEnv("THUMBNAIL_DIR", "./thumbnails"),
		DataDir:      getEnv("DATA_DIR", "./data"),
		JWTSecret:    getEnv("JWT_SECRET", "streamlet-secret-change-me"),
		Username:     getEnv("AUTH_USER", "admin"),
		Password:     getEnv("AUTH_PASS", "admin123"),
		Env:          getEnv("ENV", "development"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
