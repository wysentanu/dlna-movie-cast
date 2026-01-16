package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wysentanu/dlna-movie-cast/internal/config"
	"github.com/wysentanu/dlna-movie-cast/internal/dlna"
	"github.com/wysentanu/dlna-movie-cast/internal/library"
	"github.com/wysentanu/dlna-movie-cast/internal/transcoder"
)

// API holds all the API dependencies
type API struct {
	config        *config.Config
	library       *library.Library
	ssdp          *dlna.SSDPServer
	upnp          *dlna.UPnPHandler
	contentDir    *dlna.ContentDirectoryService
	avTransport   *dlna.AVTransportController
	streamHandler *transcoder.StreamHandler
	serverAddr    string
}

// NewAPI creates a new API instance
func NewAPI(cfg *config.Config, lib *library.Library, ssdp *dlna.SSDPServer, serverAddr string) (*API, error) {
	streamHandler, err := transcoder.NewStreamHandler(cfg, lib)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize stream handler: %w", err)
	}

	return &API{
		config:        cfg,
		library:       lib,
		ssdp:          ssdp,
		upnp:          dlna.NewUPnPHandler(ssdp.GetUUID(), cfg.DLNAFriendlyName, serverAddr),
		contentDir:    dlna.NewContentDirectoryService(lib, serverAddr),
		avTransport:   dlna.NewAVTransportController(),
		streamHandler: streamHandler,
		serverAddr:    serverAddr,
	}, nil
}

// SetupRoutes registers all HTTP routes
func (a *API) SetupRoutes(mux *http.ServeMux) {
	// Enable CORS middleware
	corsHandler := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			h(w, r)
		}
	}

	// REST API routes
	mux.HandleFunc("/api/movies", corsHandler(a.handleMovies))
	mux.HandleFunc("/api/movies/", corsHandler(a.handleMovie))
	mux.HandleFunc("/api/devices", corsHandler(a.handleDevices))
	mux.HandleFunc("/api/devices/refresh", corsHandler(a.handleRefreshDevices))
	mux.HandleFunc("/api/cast", corsHandler(a.handleCast))
	mux.HandleFunc("/api/cast/control", corsHandler(a.handleCastControl))
	mux.HandleFunc("/api/scan", corsHandler(a.handleScan))

	// Streaming routes
	mux.HandleFunc("/stream/", a.streamHandler.ServeHTTP)

	// DLNA/UPnP routes
	mux.HandleFunc("/dlna/device.xml", a.upnp.ServeDeviceDescription)
	mux.HandleFunc("/dlna/ContentDirectory.xml", a.upnp.ServeContentDirectorySCPD)
	mux.HandleFunc("/dlna/ConnectionManager.xml", a.upnp.ServeConnectionManagerSCPD)
	mux.HandleFunc("/dlna/ContentDirectory/control", a.contentDir.HandleControl)
	mux.HandleFunc("/dlna/ConnectionManager/control", a.handleConnectionManagerControl)
}

// handleMovies handles GET /api/movies
func (a *API) handleMovies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	movies := a.library.GetAllMovies()
	summaries := make([]library.MovieSummary, 0, len(movies))
	for _, movie := range movies {
		summaries = append(summaries, movie.ToSummary())
	}

	respondJSON(w, summaries)
}

// handleMovie handles /api/movies/{id}
func (a *API) handleMovie(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract movie ID from path
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	movieID := parts[2]

	// Check if requesting thumbnail
	if len(parts) >= 4 && parts[3] == "thumbnail" {
		a.handleMovieThumbnail(w, r, movieID)
		return
	}

	movie, err := a.library.GetMovie(movieID)
	if err != nil {
		http.Error(w, "Movie not found", http.StatusNotFound)
		return
	}

	// Add stream URL to response
	type MovieResponse struct {
		*library.Movie
		StreamURL string `json:"stream_url"`
	}

	response := MovieResponse{
		Movie:     movie,
		StreamURL: a.serverAddr + "/stream/" + movie.ID,
	}

	respondJSON(w, response)
}

// handleMovieThumbnail serves movie thumbnails
func (a *API) handleMovieThumbnail(w http.ResponseWriter, r *http.Request, movieID string) {
	movie, err := a.library.GetMovie(movieID)
	if err != nil {
		http.Error(w, "Movie not found", http.StatusNotFound)
		return
	}

	if movie.ThumbnailPath == "" {
		http.Error(w, "Thumbnail not available", http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, movie.ThumbnailPath)
}

// handleDevices handles GET /api/devices
func (a *API) handleDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	devices := a.ssdp.GetDevices()
	respondJSON(w, devices)
}

// handleRefreshDevices handles POST /api/devices/refresh
func (a *API) handleRefreshDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	a.ssdp.RefreshDevices()

	// Also try to add known TCL TV if not found (workaround for unresponsive TVs)
	// This was discovered earlier at 192.168.110.65
	knownDevices := []struct {
		ip   string
		port string
		path string
		name string
	}{
		{"192.168.110.65", "49152", "/tvrenderdesc.xml", "TCL TV"},
	}

	for _, kd := range knownDevices {
		a.ssdp.AddManualDevice(kd.ip, kd.port, kd.path, kd.name)
	}

	respondJSON(w, map[string]string{"status": "ok"})
}

// CastRequest represents a request to cast a movie
type CastRequest struct {
	MovieID       string `json:"movie_id"`
	DeviceUUID    string `json:"device_uuid"`
	SubtitlePath  string `json:"subtitle_path,omitempty"`
	SubtitleIndex int    `json:"subtitle_index,omitempty"`
	Transcode     bool   `json:"transcode,omitempty"`
}

// handleCast handles POST /api/cast
func (a *API) handleCast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get movie
	movie, err := a.library.GetMovie(req.MovieID)
	if err != nil {
		http.Error(w, "Movie not found", http.StatusNotFound)
		return
	}

	// Get device
	device, err := a.ssdp.GetDevice(req.DeviceUUID)
	if err != nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// Build stream URL
	params := []string{}

	isTranscoding := req.Transcode || req.SubtitlePath != "" || req.SubtitleIndex > 0

	var streamURL string
	if isTranscoding {
		// Use HLS for transcoding
		streamURL = a.serverAddr + "/stream/" + movie.ID + "/hls/playlist.m3u8"
		params = append(params, "transcode=1")
	} else {
		// Direct stream
		streamURL = a.serverAddr + "/stream/" + movie.ID
	}

	if req.SubtitlePath != "" {
		params = append(params, "subtitle="+url.QueryEscape(req.SubtitlePath))
	}
	if req.SubtitleIndex > 0 {
		params = append(params, "subtitle_index="+string(rune(req.SubtitleIndex+'0')))
	}

	if len(params) > 0 {
		streamURL += "?" + strings.Join(params, "&")
	}

	// Set URI on device
	if err := a.avTransport.SetAVTransportURI(device, streamURL, movie.Title); err != nil {
		respondJSON(w, map[string]string{"error": err.Error()})
		return
	}

	// Start playback
	if err := a.avTransport.Play(device); err != nil {
		respondJSON(w, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, map[string]string{"status": "playing", "stream_url": streamURL})
}

// CastControlRequest represents a playback control request
type CastControlRequest struct {
	DeviceUUID string `json:"device_uuid"`
	Action     string `json:"action"`             // play, pause, stop, seek
	Position   string `json:"position,omitempty"` // For seek: HH:MM:SS
}

// handleCastControl handles POST /api/cast/control
func (a *API) handleCastControl(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Get playback status
		deviceUUID := r.URL.Query().Get("device_uuid")
		if deviceUUID == "" {
			http.Error(w, "device_uuid required", http.StatusBadRequest)
			return
		}

		device, err := a.ssdp.GetDevice(deviceUUID)
		if err != nil {
			http.Error(w, "Device not found", http.StatusNotFound)
			return
		}

		state, err := a.avTransport.GetPositionInfo(device)
		if err != nil {
			respondJSON(w, map[string]string{"error": err.Error()})
			return
		}

		transportInfo, _ := a.avTransport.GetTransportInfo(device)
		if transportInfo != nil {
			state.TransportState = transportInfo.TransportState
		}

		respondJSON(w, state)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CastControlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	device, err := a.ssdp.GetDevice(req.DeviceUUID)
	if err != nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	switch req.Action {
	case "play":
		err = a.avTransport.Play(device)
	case "pause":
		err = a.avTransport.Pause(device)
	case "stop":
		err = a.avTransport.Stop(device)
	case "seek":
		if req.Position == "" {
			http.Error(w, "position required for seek", http.StatusBadRequest)
			return
		}
		err = a.avTransport.Seek(device, req.Position)
	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
		return
	}

	if err != nil {
		respondJSON(w, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, map[string]string{"status": "ok"})
}

// handleScan handles POST /api/scan
func (a *API) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := a.library.Scan(ctx); err != nil {
			// Log error - can't send to client since we're async
			println("Scan error:", err.Error())
		} else {
			println("Scan completed successfully")
		}
		a.contentDir.IncrementUpdateID()
	}()

	respondJSON(w, map[string]string{"status": "scanning"})
}

// handleConnectionManagerControl handles ConnectionManager SOAP requests
func (a *API) handleConnectionManagerControl(w http.ResponseWriter, r *http.Request) {
	soapAction := r.Header.Get("SOAPAction")

	var response string
	switch {
	case strings.Contains(soapAction, "GetProtocolInfo"):
		response = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:GetProtocolInfoResponse xmlns:u="urn:schemas-upnp-org:service:ConnectionManager:1">
<Source>http-get:*:video/mp4:*,http-get:*:video/x-matroska:*,http-get:*:video/x-msvideo:*,http-get:*:video/webm:*</Source>
<Sink></Sink>
</u:GetProtocolInfoResponse>
</s:Body>
</s:Envelope>`
	case strings.Contains(soapAction, "GetCurrentConnectionIDs"):
		response = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:GetCurrentConnectionIDsResponse xmlns:u="urn:schemas-upnp-org:service:ConnectionManager:1">
<ConnectionIDs>0</ConnectionIDs>
</u:GetCurrentConnectionIDsResponse>
</s:Body>
</s:Envelope>`
	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.Write([]byte(response))
}

// respondJSON sends a JSON response
func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
