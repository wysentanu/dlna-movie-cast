package transcoder

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/wysentanu/dlna-movie-cast/internal/config"
	"github.com/wysentanu/dlna-movie-cast/internal/library"
)

// StreamHandler handles HTTP streaming of media files
type StreamHandler struct {
	config     *config.Config
	library    *library.Library
	transcoder *Transcoder
	hlsManager *HLSManager
}

// NewStreamHandler creates a new stream handler
func NewStreamHandler(cfg *config.Config, lib *library.Library) (*StreamHandler, error) {
	// Use RAM-based storage for HLS segments to avoid disk footprint
	// Linux: /dev/shm is a tmpfs (RAM disk)
	// macOS/other: /tmp is often RAM-based or cleared on reboot
	var hlsDir string
	if _, err := os.Stat("/dev/shm"); err == nil {
		// Linux with tmpfs available
		hlsDir = "/dev/shm/dlna-movie-cast-hls"
	} else {
		// macOS or systems without /dev/shm - use /tmp
		hlsDir = "/tmp/dlna-movie-cast-hls"
	}

	log.Printf("HLS segments directory: %s (RAM-based)", hlsDir)

	hlsManager, err := NewHLSManager(hlsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create HLS manager: %w", err)
	}

	return &StreamHandler{
		config:     cfg,
		library:    lib,
		transcoder: NewTranscoder(cfg),
		hlsManager: hlsManager,
	}, nil
}

// ServeHTTP handles HTTP requests for streaming
func (h *StreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Path: /stream/{id} or /stream/{id}/hls/playlist.m3u8 or /stream/{id}/hls/segment_xxx.ts
	// parts[0] = stream, parts[1] = id, parts[2] = hls (optional), etc.
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 {
		http.Error(w, "Invalid stream path", http.StatusBadRequest)
		return
	}
	movieID := parts[1]

	// Check if this is an HLS request
	if len(parts) >= 3 && parts[2] == "hls" {
		if len(parts) < 4 {
			http.Error(w, "Invalid HLS path", http.StatusBadRequest)
			return
		}
		filename := parts[3]
		if strings.HasSuffix(filename, ".m3u8") {
			h.serveHLSPlaylist(w, r, movieID)
		} else if strings.HasSuffix(filename, ".ts") {
			h.serveHLSSegment(w, r, movieID, filename)
		} else {
			http.Error(w, "Invalid HLS file", http.StatusBadRequest)
		}
		return
	}

	movie, err := h.library.GetMovie(movieID)
	if err != nil {
		http.Error(w, "Movie not found", http.StatusNotFound)
		return
	}

	// Check query parameters
	transcode := r.URL.Query().Get("transcode") == "1"
	subtitlePath := r.URL.Query().Get("subtitle")
	subtitleIndexStr := r.URL.Query().Get("subtitle_index")
	format := r.URL.Query().Get("format")

	var subtitleIndex int = -1
	if subtitleIndexStr != "" {
		subtitleIndex, _ = strconv.Atoi(subtitleIndexStr)
	}

	// Determine if transcoding is needed
	needsTranscode := transcode || subtitlePath != "" || subtitleIndex >= 0
	needsTranscode = needsTranscode || h.transcoder.NeedsTranscode(movie, subtitlePath != "" || subtitleIndex >= 0)

	if format == "hls" {
		// Redirect to HLS playlist
		// We preserve query params but remove format=hls to avoid loops if logic changes
		q := r.URL.Query()
		q.Del("format")

		hlsURL := fmt.Sprintf("/stream/%s/hls/playlist.m3u8", movieID)
		if len(q) > 0 {
			hlsURL += "?" + q.Encode()
		}

		http.Redirect(w, r, hlsURL, http.StatusTemporaryRedirect)
		return
	}

	if needsTranscode {
		h.serveTranscodedStream(w, r, movie, subtitlePath, subtitleIndex)
	} else {
		h.serveDirectStream(w, r, movie)
	}
}

func (h *StreamHandler) serveHLSPlaylist(w http.ResponseWriter, r *http.Request, movieID string) {
	// Get or create session
	session := h.hlsManager.GetSession(movieID)

	if session == nil {
		log.Printf("[HLS] Creating session for movie %s", movieID)

		// Start new session
		movie, err := h.library.GetMovie(movieID)
		if err != nil {
			http.Error(w, "Movie not found", http.StatusNotFound)
			return
		}

		session, err = h.hlsManager.CreateSession(movieID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create HLS session: %v", err), http.StatusInternalServerError)
			return
		}

		// Prepare transcode options
		subtitlePath := r.URL.Query().Get("subtitle")
		subtitleIndexStr := r.URL.Query().Get("subtitle_index")
		var subtitleIndex int = -1
		if subtitleIndexStr != "" {
			subtitleIndex, _ = strconv.Atoi(subtitleIndexStr)
		}

		opts := DefaultOptions(h.config)
		opts.SubtitlePath = subtitlePath
		opts.SubtitleIndex = subtitleIndex
		opts.Format = "hls"
		opts.OutputPath = session.Dir

		// Use background context so transcode doesn't stop when request ends
		ctx := context.Background()

		// Start transcoding process
		process, err := h.transcoder.StartHLSTranscode(ctx, movie, opts)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to start HLS transcoding: %v", err), http.StatusInternalServerError)
			return
		}

		session.Process = process
		log.Printf("[HLS] Started transcoding session %s", session.ID)

		// Wait for the playlist to be created
		for i := 0; i < 30; i++ {
			if _, err := os.Stat(h.hlsManager.GetPlaylistPath(session.ID)); err == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	playlistPath := h.hlsManager.GetPlaylistPath(session.ID)

	// Check if playlist exists
	if _, err := os.Stat(playlistPath); os.IsNotExist(err) {
		http.Error(w, "Playlist not ready yet", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, playlistPath)
}

func (h *StreamHandler) serveHLSSegment(w http.ResponseWriter, r *http.Request, movieID, filename string) {
	session := h.hlsManager.GetSession(movieID)
	if session == nil {
		http.Error(w, "Session expired", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "max-age=3600")

	if err := h.hlsManager.CopySegment(session.ID, filename, w); err != nil {
		http.Error(w, "Segment not found", http.StatusNotFound)
		return
	}
}

// serveDirectStream serves the video file directly with range support
func (h *StreamHandler) serveDirectStream(w http.ResponseWriter, r *http.Request, movie *library.Movie) {
	file, err := os.Open(movie.FilePath)
	if err != nil {
		http.Error(w, "Failed to open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "Failed to stat file", http.StatusInternalServerError)
		return
	}

	// Set content type based on file extension
	contentType := h.getContentType(movie.FilePath)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")

	// Handle range requests
	http.ServeContent(w, r, movie.Title, stat.ModTime(), file)
}

// serveTranscodedStream serves a transcoded video stream
func (h *StreamHandler) serveTranscodedStream(w http.ResponseWriter, r *http.Request, movie *library.Movie, subtitlePath string, subtitleIndex int) {
	opts := DefaultOptions(h.config)
	opts.SubtitlePath = subtitlePath
	opts.SubtitleIndex = subtitleIndex

	// Parse start time from query
	if startStr := r.URL.Query().Get("start"); startStr != "" {
		if start, err := strconv.Atoi(startStr); err == nil {
			opts.StartTime = start
		}
	}

	// Start transcoding
	reader, err := h.transcoder.Transcode(r.Context(), movie, opts)
	if err != nil {
		http.Error(w, fmt.Sprintf("Transcoding failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	// Set headers for streaming
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Cache-Control", "no-cache")

	// DLNA-specific headers
	w.Header().Set("transferMode.dlna.org", "Streaming")
	w.Header().Set("contentFeatures.dlna.org", "DLNA.ORG_OP=01;DLNA.ORG_CI=1;DLNA.ORG_FLAGS=01700000000000000000000000000000")

	// Stream the transcoded output
	_, err = io.Copy(w, reader)
	if err != nil {
		// Client likely disconnected, which is normal
		return
	}
}

// getContentType returns the MIME type for a video file
func (h *StreamHandler) getContentType(path string) string {
	ext := strings.ToLower(path)
	switch {
	case strings.HasSuffix(ext, ".mp4"), strings.HasSuffix(ext, ".m4v"):
		return "video/mp4"
	case strings.HasSuffix(ext, ".mkv"):
		return "video/x-matroska"
	case strings.HasSuffix(ext, ".avi"):
		return "video/x-msvideo"
	case strings.HasSuffix(ext, ".webm"):
		return "video/webm"
	case strings.HasSuffix(ext, ".mov"):
		return "video/quicktime"
	case strings.HasSuffix(ext, ".wmv"):
		return "video/x-ms-wmv"
	case strings.HasSuffix(ext, ".ts"), strings.HasSuffix(ext, ".m2ts"):
		return "video/mp2t"
	default:
		return "video/mp4"
	}
}

// GetStreamURL returns the URL for streaming a movie
func (h *StreamHandler) GetStreamURL(baseURL, movieID string, transcode bool, subtitlePath string) string {
	url := fmt.Sprintf("%s/stream/%s", baseURL, movieID)

	params := []string{}
	if transcode {
		params = append(params, "transcode=1")
	}
	if subtitlePath != "" {
		params = append(params, "subtitle="+subtitlePath)
	}

	if len(params) > 0 {
		url += "?" + strings.Join(params, "&")
	}

	return url
}
