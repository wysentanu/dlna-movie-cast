package library

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wysentanu/dlna-movie-cast/internal/config"
	_ "modernc.org/sqlite"
)

// Library manages the media library
type Library struct {
	config *config.Config
	db     *sql.DB
	mu     sync.RWMutex
	movies map[string]*Movie
}

// NewLibrary creates a new library instance
func NewLibrary(cfg *config.Config) (*Library, error) {
	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	lib := &Library{
		config: cfg,
		db:     db,
		movies: make(map[string]*Movie),
	}

	if err := lib.initDB(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return lib, nil
}

// initDB creates the database schema
func (l *Library) initDB() error {
	schema := `
	CREATE TABLE IF NOT EXISTS movies (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		year INTEGER,
		duration INTEGER,
		file_path TEXT UNIQUE NOT NULL,
		file_size INTEGER,
		video_codec TEXT,
		video_width INTEGER,
		video_height INTEGER,
		video_bitrate INTEGER,
		audio_codec TEXT,
		audio_channels INTEGER,
		subtitles TEXT,
		thumbnail_path TEXT,
		added_at DATETIME,
		modified_at DATETIME
	);
	CREATE INDEX IF NOT EXISTS idx_movies_title ON movies(title);
	CREATE INDEX IF NOT EXISTS idx_movies_file_path ON movies(file_path);
	`
	_, err := l.db.Exec(schema)
	return err
}

// Scan scans the media directories for movies
func (l *Library) Scan(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, mediaPath := range l.config.MediaPaths {
		if err := l.scanDirectory(ctx, mediaPath); err != nil {
			return err
		}
	}

	return l.loadFromDB()
}

// scanDirectory recursively scans a directory for video files
func (l *Library) scanDirectory(ctx context.Context, dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !l.isVideoExtension(ext) {
			return nil
		}

		return l.processVideoFile(ctx, path, info)
	})
}

// isVideoExtension checks if the extension is a supported video format
func (l *Library) isVideoExtension(ext string) bool {
	for _, e := range l.config.MediaExtensions {
		if ext == e {
			return true
		}
	}
	return false
}

// processVideoFile processes a single video file
func (l *Library) processVideoFile(ctx context.Context, path string, info os.FileInfo) error {
	id := l.generateID(path)

	// Check if already in database and up-to-date
	existing, err := l.getMovieFromDB(id)
	if err == nil && existing.ModifiedAt.Equal(info.ModTime()) {
		return nil // No changes
	}

	// Extract metadata using ffprobe
	movie, err := l.extractMetadata(ctx, path, info)
	if err != nil {
		// Skip this file but continue scanning
		return nil
	}

	// Find external subtitles
	movie.Subtitles = append(movie.Subtitles, l.findExternalSubtitles(path)...)

	// Generate thumbnail
	if l.config.ThumbnailDir != "" {
		thumbPath := filepath.Join(l.config.ThumbnailDir, id+".jpg")
		if err := l.generateThumbnail(ctx, path, thumbPath, movie.Duration); err == nil {
			movie.ThumbnailPath = thumbPath
		}
	}

	// Save to database
	return l.saveMovie(movie)
}

// generateID creates a unique ID for a file based on its path
func (l *Library) generateID(path string) string {
	hash := sha256.Sum256([]byte(path))
	return hex.EncodeToString(hash[:8])
}

// extractMetadata uses ffprobe to extract video metadata
func (l *Library) extractMetadata(ctx context.Context, path string, info os.FileInfo) (*Movie, error) {
	cmd := exec.CommandContext(ctx, l.config.FFprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var probeData struct {
		Format struct {
			Duration string `json:"duration"`
			Size     string `json:"size"`
			BitRate  string `json:"bit_rate"`
		} `json:"format"`
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			BitRate   string `json:"bit_rate"`
			Channels  int    `json:"channels"`
			Index     int    `json:"index"`
			Tags      struct {
				Language string `json:"language"`
				Title    string `json:"title"`
			} `json:"tags"`
		} `json:"streams"`
	}

	if err := json.Unmarshal(output, &probeData); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	movie := &Movie{
		ID:         l.generateID(path),
		FilePath:   path,
		FileSize:   info.Size(),
		AddedAt:    time.Now(),
		ModifiedAt: info.ModTime(),
	}

	// Parse title from filename
	movie.Title, movie.Year = l.parseFilename(filepath.Base(path))

	// Parse duration
	if dur, err := strconv.ParseFloat(probeData.Format.Duration, 64); err == nil {
		movie.Duration = int(dur)
	}

	// Extract stream information
	for _, stream := range probeData.Streams {
		switch stream.CodecType {
		case "video":
			if movie.VideoCodec == "" {
				movie.VideoCodec = stream.CodecName
				movie.VideoWidth = stream.Width
				movie.VideoHeight = stream.Height
				if br, err := strconv.ParseInt(stream.BitRate, 10, 64); err == nil {
					movie.VideoBitrate = br
				}
			}
		case "audio":
			if movie.AudioCodec == "" {
				movie.AudioCodec = stream.CodecName
				movie.AudioChannels = stream.Channels
			}
		case "subtitle":
			movie.Subtitles = append(movie.Subtitles, Subtitle{
				Index:      stream.Index,
				Language:   stream.Tags.Language,
				Title:      stream.Tags.Title,
				IsExternal: false,
				Format:     stream.CodecName,
			})
		}
	}

	return movie, nil
}

// parseFilename extracts title and year from filename
func (l *Library) parseFilename(filename string) (string, int) {
	// Remove extension
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Try to extract year (e.g., "Movie Name (2023)" or "Movie.Name.2023")
	yearRegex := regexp.MustCompile(`[\.\s\(]*((?:19|20)\d{2})[\.\s\)]*`)
	matches := yearRegex.FindStringSubmatch(name)

	var year int
	if len(matches) > 1 {
		year, _ = strconv.Atoi(matches[1])
		name = yearRegex.ReplaceAllString(name, " ")
	}

	// Clean up title
	name = strings.ReplaceAll(name, ".", " ")
	name = strings.ReplaceAll(name, "_", " ")
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")
	name = strings.TrimSpace(name)

	return name, year
}

// findExternalSubtitles looks for external subtitle files
func (l *Library) findExternalSubtitles(videoPath string) []Subtitle {
	var subtitles []Subtitle
	dir := filepath.Dir(videoPath)
	baseName := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))

	// Look for .srt, .ass, .ssa, .sub files
	subExts := []string{".srt", ".ass", ".ssa", ".sub", ".vtt"}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return subtitles
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))

		// Check if it's a subtitle file
		isSubtitle := false
		for _, subExt := range subExts {
			if ext == subExt {
				isSubtitle = true
				break
			}
		}
		if !isSubtitle {
			continue
		}

		// Check if it matches the video file
		subBase := strings.TrimSuffix(name, ext)
		if !strings.HasPrefix(subBase, baseName) {
			continue
		}

		// Extract language from filename (e.g., "movie.en.srt")
		lang := ""
		suffix := strings.TrimPrefix(subBase, baseName)
		suffix = strings.Trim(suffix, "._-")
		if len(suffix) == 2 || len(suffix) == 3 {
			lang = suffix
		}

		subtitles = append(subtitles, Subtitle{
			Index:      len(subtitles),
			Language:   lang,
			FilePath:   filepath.Join(dir, name),
			IsExternal: true,
			Format:     strings.TrimPrefix(ext, "."),
		})
	}

	return subtitles
}

// generateThumbnail generates a thumbnail for the video
func (l *Library) generateThumbnail(ctx context.Context, videoPath, thumbPath string, duration int) error {
	// Seek to 10% of the video or 30 seconds, whichever is less
	seekTime := duration / 10
	if seekTime > 30 {
		seekTime = 30
	}
	if seekTime < 1 {
		seekTime = 1
	}

	cmd := exec.CommandContext(ctx, l.config.FFmpegPath,
		"-ss", strconv.Itoa(seekTime),
		"-i", videoPath,
		"-vframes", "1",
		"-vf", "scale=320:-1",
		"-y",
		thumbPath,
	)

	return cmd.Run()
}

// saveMovie saves a movie to the database
func (l *Library) saveMovie(movie *Movie) error {
	subtitlesJSON, _ := json.Marshal(movie.Subtitles)

	_, err := l.db.Exec(`
		INSERT OR REPLACE INTO movies (
			id, title, year, duration, file_path, file_size,
			video_codec, video_width, video_height, video_bitrate,
			audio_codec, audio_channels, subtitles, thumbnail_path,
			added_at, modified_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		movie.ID, movie.Title, movie.Year, movie.Duration, movie.FilePath, movie.FileSize,
		movie.VideoCodec, movie.VideoWidth, movie.VideoHeight, movie.VideoBitrate,
		movie.AudioCodec, movie.AudioChannels, string(subtitlesJSON), movie.ThumbnailPath,
		movie.AddedAt, movie.ModifiedAt,
	)

	return err
}

// getMovieFromDB retrieves a movie from the database
func (l *Library) getMovieFromDB(id string) (*Movie, error) {
	row := l.db.QueryRow(`
		SELECT id, title, year, duration, file_path, file_size,
			video_codec, video_width, video_height, video_bitrate,
			audio_codec, audio_channels, subtitles, thumbnail_path,
			added_at, modified_at
		FROM movies WHERE id = ?
	`, id)

	return l.scanMovie(row)
}

// scanMovie scans a database row into a Movie
func (l *Library) scanMovie(row *sql.Row) (*Movie, error) {
	var movie Movie
	var subtitlesJSON string
	var year, duration, width, height, channels sql.NullInt64
	var videoBitrate sql.NullInt64
	var videoCodec, audioCodec, thumbnailPath sql.NullString

	err := row.Scan(
		&movie.ID, &movie.Title, &year, &duration, &movie.FilePath, &movie.FileSize,
		&videoCodec, &width, &height, &videoBitrate,
		&audioCodec, &channels, &subtitlesJSON, &thumbnailPath,
		&movie.AddedAt, &movie.ModifiedAt,
	)
	if err != nil {
		return nil, err
	}

	if year.Valid {
		movie.Year = int(year.Int64)
	}
	if duration.Valid {
		movie.Duration = int(duration.Int64)
	}
	if videoCodec.Valid {
		movie.VideoCodec = videoCodec.String
	}
	if width.Valid {
		movie.VideoWidth = int(width.Int64)
	}
	if height.Valid {
		movie.VideoHeight = int(height.Int64)
	}
	if videoBitrate.Valid {
		movie.VideoBitrate = videoBitrate.Int64
	}
	if audioCodec.Valid {
		movie.AudioCodec = audioCodec.String
	}
	if channels.Valid {
		movie.AudioChannels = int(channels.Int64)
	}
	if thumbnailPath.Valid {
		movie.ThumbnailPath = thumbnailPath.String
	}

	if subtitlesJSON != "" {
		json.Unmarshal([]byte(subtitlesJSON), &movie.Subtitles)
	}

	return &movie, nil
}

// loadFromDB loads all movies from the database into memory
func (l *Library) loadFromDB() error {
	rows, err := l.db.Query(`
		SELECT id, title, year, duration, file_path, file_size,
			video_codec, video_width, video_height, video_bitrate,
			audio_codec, audio_channels, subtitles, thumbnail_path,
			added_at, modified_at
		FROM movies ORDER BY title
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	l.movies = make(map[string]*Movie)

	for rows.Next() {
		var movie Movie
		var subtitlesJSON string
		var year, duration, width, height, channels sql.NullInt64
		var videoBitrate sql.NullInt64
		var videoCodec, audioCodec, thumbnailPath sql.NullString

		err := rows.Scan(
			&movie.ID, &movie.Title, &year, &duration, &movie.FilePath, &movie.FileSize,
			&videoCodec, &width, &height, &videoBitrate,
			&audioCodec, &channels, &subtitlesJSON, &thumbnailPath,
			&movie.AddedAt, &movie.ModifiedAt,
		)
		if err != nil {
			continue
		}

		if year.Valid {
			movie.Year = int(year.Int64)
		}
		if duration.Valid {
			movie.Duration = int(duration.Int64)
		}
		if videoCodec.Valid {
			movie.VideoCodec = videoCodec.String
		}
		if width.Valid {
			movie.VideoWidth = int(width.Int64)
		}
		if height.Valid {
			movie.VideoHeight = int(height.Int64)
		}
		if videoBitrate.Valid {
			movie.VideoBitrate = videoBitrate.Int64
		}
		if audioCodec.Valid {
			movie.AudioCodec = audioCodec.String
		}
		if channels.Valid {
			movie.AudioChannels = int(channels.Int64)
		}
		if thumbnailPath.Valid {
			movie.ThumbnailPath = thumbnailPath.String
		}

		if subtitlesJSON != "" {
			json.Unmarshal([]byte(subtitlesJSON), &movie.Subtitles)
		}

		l.movies[movie.ID] = &movie
	}

	return nil
}

// GetAllMovies returns all movies in the library
func (l *Library) GetAllMovies() []*Movie {
	l.mu.RLock()
	defer l.mu.RUnlock()

	movies := make([]*Movie, 0, len(l.movies))
	for _, movie := range l.movies {
		movies = append(movies, movie)
	}
	return movies
}

// GetMovie returns a movie by ID
func (l *Library) GetMovie(id string) (*Movie, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	movie, ok := l.movies[id]
	if !ok {
		return nil, fmt.Errorf("movie not found: %s", id)
	}
	return movie, nil
}

// Close closes the library database connection
func (l *Library) Close() error {
	return l.db.Close()
}
