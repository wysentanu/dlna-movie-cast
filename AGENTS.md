# DLNA Movie Cast - AGENTS.md

## Project Overview

A lightweight media streaming server designed to stream movies with on-the-fly transcoding and SRT subtitle burning to DLNA-compatible TVs.

### Key Features
- **Media Library Management**: Scan, index, and serve movie files
- **DLNA/UPnP Server**: Cast content to smart TVs
- **CPU-based Transcoding**: Software encoding with libx264 (works everywhere)
- **On-the-fly Subtitle Burning**: Burn SRT subtitles into video stream
- **HLS Streaming**: Adaptive streaming for better TV compatibility
- **Optional Hardware Acceleration**: Support for VAAPI, NVENC, Rockchip MPP
- **Web-based Control**: Modern UI for browsing and controlling playback

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Media Server                            │
├──────────────────────────┬──────────────────────────────────┤
│      Frontend (Web)      │         Backend (Go)              │
│  ┌────────────────────┐  │  ┌──────────────────────────┐    │
│  │   Vite + Vanilla   │  │  │    REST API Server       │    │
│  │   HTML/CSS/JS      │◄─┼──┤    (Port 8080)           │    │
│  └────────────────────┘  │  └──────────┬───────────────┘    │
│                          │             │                     │
│                          │  ┌──────────▼───────────────┐    │
│                          │  │    Media Library         │    │
│                          │  │    (SQLite Cache)        │    │
│                          │  └──────────┬───────────────┘    │
│                          │             │                     │
│                          │  ┌──────────▼───────────────┐    │
│                          │  │    DLNA/UPnP Server      │    │
│                          │  │    (SSDP + SOAP)         │    │
│                          │  └──────────┬───────────────┘    │
│                          │             │                     │
│                          │  ┌──────────▼───────────────┐    │
│                          │  │    FFmpeg Transcoder     │    │
│                          │  │    (CPU / HW Accel)      │    │
│                          │  └──────────────────────────┘    │
└──────────────────────────┴──────────────────────────────────┘
                                      │
                                      ▼ DLNA/HLS
                           ┌─────────────────────┐
                           │      Smart TV       │
                           │   (DLNA Renderer)   │
                           └─────────────────────┘
```

## Technology Stack

### Backend (Go)
- **Language**: Go 1.21+
- **HTTP Server**: Standard library `net/http`
- **Database**: SQLite (via `modernc.org/sqlite`)
- **DLNA/UPnP**: Custom implementation with SSDP multicast
- **Transcoding**: FFmpeg with HLS output (subprocess)
- **Session Management**: UUID-based HLS session tracking

### Frontend (Web)
- **Build Tool**: Vite
- **Framework**: Vanilla JavaScript (no React/Vue)
- **Styling**: Custom CSS with modern design patterns
- **Icons**: Inline SVG icons

### System Requirements
- **OS**: Linux, macOS, or Windows
- **FFmpeg**: Standard FFmpeg with libx264, libass
- **Optional**: Hardware encoder support (VAAPI, NVENC, or Rockchip MPP)

## Directory Structure

```
dlna-movie-cast/
├── README.md                    # Project README for GitHub
├── LICENSE                      # MIT License
├── AGENTS.md                    # This file - development documentation
├── .gitignore                   # Git ignore rules
├── server                       # Compiled Go binary (git-ignored)
├── backend/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go          # Application entry point
│   ├── internal/
│   │   ├── config/
│   │   │   └── config.go        # Configuration management
│   │   ├── library/
│   │   │   ├── library.go       # Media scanning and indexing
│   │   │   └── movie.go         # Movie entity definition
│   │   ├── dlna/
│   │   │   ├── ssdp.go          # SSDP discovery protocol
│   │   │   ├── upnp.go          # UPnP device/service XML
│   │   │   ├── contentdirectory.go  # ContentDirectory SOAP service
│   │   │   └── avtransport.go   # AVTransport SOAP controller
│   │   ├── transcoder/
│   │   │   ├── transcoder.go    # FFmpeg transcoding pipeline
│   │   │   ├── stream.go        # HTTP streaming handler (Direct + HLS)
│   │   │   └── hls.go           # HLS session management
│   │   └── api/
│   │       └── api.go           # REST API & route handlers
│   └── go.mod                   # Go module definition
└── frontend/
    ├── index.html               # HTML entry point
    ├── package.json             # NPM dependencies
    ├── vite.config.js           # Vite configuration
    └── src/
        ├── main.js              # Application logic & UI
        ├── api.js               # Backend API client
        └── index.css            # Styles (design system)
```

## Key Concepts

### DLNA/UPnP Protocol
1. **SSDP Discovery**: Server advertises itself via multicast on `239.255.255.250:1900`
2. **Device Description**: XML describing the server capabilities
3. **ContentDirectory**: Service for browsing media library
4. **AVTransport**: Service for controlling playback on renderers

### Transcoding Strategy
- **Direct Play**: When video is compatible with TV, stream directly (no transcoding)
- **HLS Transcode**: When video needs conversion or subtitle burning, use HLS streaming
- **Software Encoding**: Uses libx264 by default (works on any system)
- **Hardware Acceleration**: Automatically detected and used when available

### HLS Streaming Architecture
```
Client Request → HLSManager → FFmpeg (HLS output) → /tmp or /dev/shm
       ↓                              ↓
   playlist.m3u8              segment_000.ts, segment_001.ts, ...
       ↓                              ↓
   TV fetches playlist → TV requests segments → Playback
```

**HLS Session Lifecycle:**
1. First playlist request creates a new session
2. FFmpeg starts transcoding to temporary directory
3. Segments are written to RAM-based storage (`/dev/shm` on Linux, `/tmp` on macOS)
4. Sessions auto-expire after 10 minutes of inactivity
5. Cleanup routine removes old segments

### Subtitle Burning Pipeline
```
Input Video → Decode → subtitle filter → Encode → HLS Segments → TV
     ↑          ↑           ↑              ↑            ↑
  (file)    (decoder)   (libass)      (libx264)    (m3u8+ts)
```

## API Endpoints

### REST API (Port 8080)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/movies` | List all movies |
| GET | `/api/movies/{id}` | Get movie details with stream URL |
| GET | `/api/movies/{id}/thumbnail` | Get movie thumbnail |
| GET | `/api/devices` | List discovered DLNA devices |
| POST | `/api/devices/refresh` | Trigger SSDP device search |
| POST | `/api/cast` | Cast movie to DLNA device |
| POST | `/api/cast/control` | Control playback (play/pause/stop/seek) |
| GET | `/api/cast/control?device_uuid=xxx` | Get playback status |
| POST | `/api/scan` | Trigger library rescan |

### Streaming Endpoints

| Endpoint | Description |
|----------|-------------|
| `/stream/{id}` | Direct stream (no transcode) |
| `/stream/{id}?transcode=1&subtitle=path` | Legacy transcoded MP4 stream |
| `/stream/{id}/hls/playlist.m3u8` | HLS playlist (preferred for transcoding) |
| `/stream/{id}/hls/segment_XXX.ts` | HLS segment files |

### DLNA/UPnP Endpoints

| Endpoint | Description |
|----------|-------------|
| `/dlna/device.xml` | Device description XML |
| `/dlna/ContentDirectory.xml` | ContentDirectory SCPD |
| `/dlna/ConnectionManager.xml` | ConnectionManager SCPD |
| `/dlna/ContentDirectory/control` | ContentDirectory SOAP actions |
| `/dlna/ConnectionManager/control` | ConnectionManager SOAP actions |

## Configuration

### Environment Variables
```bash
MEDIA_PATH=/media/movies           # Path to media library (required)
SERVER_PORT=8080                   # HTTP server port
FFMPEG_PATH=ffmpeg                 # Path to FFmpeg binary
FFPROBE_PATH=ffprobe               # Path to FFprobe binary
VIDEO_BITRATE=8M                   # Target video bitrate
AUDIO_BITRATE=192k                 # Target audio bitrate
```

## Logging

The application uses consistent log prefixes for easy filtering:

| Prefix | Component | Description |
|--------|-----------|-------------|
| `[SSDP]` | SSDP/DLNA | Device discovery and network events |
| `[HLS]` | HLS Manager | Session creation, transcoding start/stop |

**Example Log Output:**
```
[SSDP] Discovered device: uuid:xxx at http://192.168.1.100:49152/
[SSDP] Added manual device: TCL TV at http://192.168.1.65:49152/
[HLS] Creating session for movie abc123
[HLS] Started transcoding session xyz789
[HLS] Cleaned up expired session xyz789 for movie abc123
```

## Docker Deployment

### Standard Run
```bash
docker-compose up -d
```

### With Rockchip Hardware Acceleration
1. Uncomment the Rockchip devices section in `docker-compose.yml`:
   ```yaml
   devices:
     - /dev/mpp_service:/dev/mpp_service
     - /dev/rga:/dev/rga
     - /dev/dri:/dev/dri
   ```
2. Note: The standard image uses `alpine` ffmpeg. For Rockchip support, you may need to mount a custom `ffmpeg` binary or rebuild the image with a Rockchip-enabled base.

## Development Workflow

### Backend Development
```bash
cd backend
go mod tidy
go run cmd/server/main.go
```

### Frontend Development
```bash
cd frontend
npm install
npm run dev  # Starts Vite dev server with HMR
```

### Building for Production
```bash
# Backend (native)
cd backend
go build -o ../server ./cmd/server/main.go

# Backend (cross-compile for Linux ARM64)
GOOS=linux GOARCH=arm64 go build -o ../server ./cmd/server/main.go

# Frontend
cd frontend
npm run build  # Output in dist/
```

### Running Production
```bash
# Start server with media path
MEDIA_PATH=/path/to/movies ./server
```

## Testing

### Unit Tests
```bash
cd backend && go test ./...
```

### Integration Testing
1. Ensure FFmpeg is installed
2. Add sample media files to MEDIA_PATH
3. Start the server
4. Open browser to `http://localhost:8080`
5. Verify DLNA device discovery
6. Test casting to TV (with and without subtitles)

## Hardware Acceleration (Optional)

The server uses **software encoding (libx264) by default**, which works on any system.
Hardware acceleration is automatically detected and enabled when available.

### Supported Hardware Encoders

| Platform | Encoder | Detection |
|----------|---------|-----------|
| Intel/AMD (Linux) | VAAPI | Auto-detected via ffmpeg |
| NVIDIA | NVENC | Auto-detected via ffmpeg |
| Rockchip ARM SBCs | MPP | Checks for `/dev/mpp_service` |

### Enabling Hardware Acceleration

Hardware acceleration is automatically enabled if the system supports it.
No configuration is required - the server probes for available encoders.

To verify hardware support:
```bash
# Check available hardware accelerators
ffmpeg -hwaccels

# Check available encoders
ffmpeg -encoders | grep -E 'h264|hevc'
```

## Common Issues

### TV Shows "File Not Supported"
- This is usually a codec/container compatibility issue
- The server now uses HLS streaming which works better with most TVs
- Ensure subtitle burning is using HLS mode (automatic for cast with subtitles)

### DLNA Device Not Found
- Ensure multicast is not blocked by firewall
- TV must be on the same network/subnet
- Try "Refresh Devices" button multiple times
- Use the manual device fallback if SSDP discovery fails

### HLS Playback Starts Then Stops
- Check that segments are being created in `/tmp/dlna-movie-cast-hls/`
- Verify FFmpeg process is still running
- Check server logs for FFmpeg errors

### Subtitles Not Burning
- Ensure SRT file is UTF-8 encoded
- Check that libass is compiled into FFmpeg
- Verify subtitle file path is accessible

### Slow Transcoding
- Software encoding on low-power devices may struggle with high bitrates
- Consider reducing VIDEO_BITRATE to `4M` or `2M`
- Enable hardware acceleration if available

## Code Style

### Go
- Use `gofmt` for formatting
- Log prefixes: `[ComponentName]` format
- Error wrapping with context
- Consistent struct field ordering

### JavaScript
- JSDoc comments for functions
- XSS protection via `escapeHtml()`
- Async/await for API calls
- Clean separation: api.js (data), main.js (UI)

## References

- [FFmpeg Documentation](https://ffmpeg.org/documentation.html)
- [UPnP Device Architecture](http://upnp.org/specs/arch/UPnP-arch-DeviceArchitecture-v1.1.pdf)
- [DLNA Guidelines](https://spirespark.com/dlna/guidelines)
- [HLS Specification](https://datatracker.ietf.org/doc/html/rfc8216)
