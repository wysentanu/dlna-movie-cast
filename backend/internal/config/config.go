package config

import (
	"os"
	"path/filepath"
	"strconv"
)

// Config holds application configuration
type Config struct {
	// Server settings
	ServerHost string
	ServerPort int

	// Media library settings
	MediaPaths      []string
	MediaExtensions []string

	// Database
	DBPath string

	// FFmpeg settings
	FFmpegPath   string
	FFprobePath  string
	VideoBitrate string
	AudioBitrate string
	Preset       string

	// DLNA settings
	DLNAFriendlyName string
	DLNAUUID         string

	// Thumbnail settings
	ThumbnailDir string
}

// DefaultConfig returns configuration with sensible defaults
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	dataDir := filepath.Join(homeDir, ".dlna-movie-cast")

	return &Config{
		ServerHost: "0.0.0.0",
		ServerPort: 8080,

		MediaPaths: []string{"/media/movies"},
		MediaExtensions: []string{
			".mkv", ".mp4", ".avi", ".mov", ".wmv",
			".m4v", ".webm", ".ts", ".m2ts",
		},

		DBPath: filepath.Join(dataDir, "library.db"),

		FFmpegPath:   "ffmpeg",
		FFprobePath:  "ffprobe",
		VideoBitrate: "2M",
		AudioBitrate: "192k",
		Preset:       "fast",

		DLNAFriendlyName: "DLNA Movie Cast",
		DLNAUUID:         "", // Will be auto-generated if empty

		ThumbnailDir: filepath.Join(dataDir, "thumbnails"),
	}
}

// LoadFromEnv loads configuration from environment variables
func (c *Config) LoadFromEnv() {
	if val := os.Getenv("MEDIA_PATH"); val != "" {
		c.MediaPaths = []string{val}
	}
	if val := os.Getenv("MEDIA_PATHS"); val != "" {
		c.MediaPaths = filepath.SplitList(val)
	}
	if val := os.Getenv("SERVER_HOST"); val != "" {
		c.ServerHost = val
	}
	if val := os.Getenv("SERVER_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			c.ServerPort = port
		}
	}
	if val := os.Getenv("FFMPEG_PATH"); val != "" {
		c.FFmpegPath = val
	}
	if val := os.Getenv("FFPROBE_PATH"); val != "" {
		c.FFprobePath = val
	}
	if val := os.Getenv("DB_PATH"); val != "" {
		c.DBPath = val
	}
	if val := os.Getenv("VIDEO_BITRATE"); val != "" {
		c.VideoBitrate = val
	}
	if val := os.Getenv("AUDIO_BITRATE"); val != "" {
		c.AudioBitrate = val
	}
	if val := os.Getenv("DLNA_FRIENDLY_NAME"); val != "" {
		c.DLNAFriendlyName = val
	}
	if val := os.Getenv("DLNA_UUID"); val != "" {
		c.DLNAUUID = val
	}
	if val := os.Getenv("THUMBNAIL_DIR"); val != "" {
		c.ThumbnailDir = val
	}
}

// EnsureDirectories creates necessary data directories
func (c *Config) EnsureDirectories() error {
	dirs := []string{
		filepath.Dir(c.DBPath),
		c.ThumbnailDir,
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}
