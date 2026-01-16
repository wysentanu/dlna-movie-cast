package library

import (
	"time"
)

// Movie represents a movie in the library
type Movie struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Year        int       `json:"year,omitempty"`
	Duration    int       `json:"duration"` // Duration in seconds
	FilePath    string    `json:"file_path"`
	FileSize    int64     `json:"file_size"`
	
	// Video info
	VideoCodec  string    `json:"video_codec"`
	VideoWidth  int       `json:"video_width"`
	VideoHeight int       `json:"video_height"`
	VideoBitrate int64    `json:"video_bitrate"`
	
	// Audio info
	AudioCodec  string    `json:"audio_codec"`
	AudioChannels int     `json:"audio_channels"`
	
	// Subtitles
	Subtitles   []Subtitle `json:"subtitles"`
	
	// Metadata
	ThumbnailPath string   `json:"thumbnail_path,omitempty"`
	AddedAt       time.Time `json:"added_at"`
	ModifiedAt    time.Time `json:"modified_at"`
}

// Subtitle represents a subtitle track
type Subtitle struct {
	Index    int    `json:"index"`
	Language string `json:"language"`
	Title    string `json:"title,omitempty"`
	FilePath string `json:"file_path,omitempty"` // For external SRT files
	IsExternal bool `json:"is_external"`
	Format   string `json:"format"` // srt, ass, subrip, etc.
}

// NeedsTranscode checks if the movie needs transcoding for a target device
func (m *Movie) NeedsTranscode(targetCodecs []string) bool {
	for _, codec := range targetCodecs {
		if m.VideoCodec == codec {
			return false
		}
	}
	return true
}

// GetExternalSubtitles returns only external subtitle files
func (m *Movie) GetExternalSubtitles() []Subtitle {
	var external []Subtitle
	for _, sub := range m.Subtitles {
		if sub.IsExternal {
			external = append(external, sub)
		}
	}
	return external
}

// MovieSummary is a lightweight version for list views
type MovieSummary struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Year          int    `json:"year,omitempty"`
	Duration      int    `json:"duration"`
	ThumbnailPath string `json:"thumbnail_path,omitempty"`
	HasSubtitles  bool   `json:"has_subtitles"`
}

// ToSummary converts a Movie to MovieSummary
func (m *Movie) ToSummary() MovieSummary {
	return MovieSummary{
		ID:            m.ID,
		Title:         m.Title,
		Year:          m.Year,
		Duration:      m.Duration,
		ThumbnailPath: m.ThumbnailPath,
		HasSubtitles:  len(m.Subtitles) > 0,
	}
}
