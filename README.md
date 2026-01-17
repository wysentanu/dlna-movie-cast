> [!WARNING]
> **Experimental Phase**: This project is currently under active development. Features may be unstable, and significant changes to the architecture or configuration may occur without notice.

# DLNA Movie Cast

A lightweight media server that streams movies to DLNA devices (like smart TVs) with on-the-fly transcoding and subtitle burning support.

## Features

- 🎬 **Media Library Scanning** - Automatically discovers video files and extracts metadata
- 📺 **DLNA/UPnP Support** - Cast to any DLNA-compatible TV or media renderer
- 🔤 **Subtitle Burning** - Real-time subtitle overlay (SRT, ASS, SSA) via FFmpeg
- ⚡ **HLS Streaming** - Adaptive streaming for better TV compatibility
- 🎨 **Modern Web UI** - Beautiful responsive interface for browsing and casting
- 🔧 **Optional Hardware Acceleration** - Support for hardware encoders (VAAPI, NVENC, Rockchip MPP)

## Requirements

- Go 1.21+
- FFmpeg with libx264
- Node.js 18+ (for frontend development)

## Quick Start

### Docker (Recommended)

```bash
docker-compose up -d
```
Access the web UI at `http://localhost`.

### Manual

**Backend:**
```bash
cd backend
go build -o ../server ./cmd/server/main.go
cd ..
MEDIA_PATH=/path/to/movies ./server
```

**Frontend:**
```bash
cd frontend
npm install
npm run dev
```

## Hardware Acceleration

Hardware acceleration is automatically detected. For Rockchip, Intel, or NVIDIA support in Docker, uncomment the relevant section in `docker-compose.yml`.

## License

MIT License - see [LICENSE](LICENSE) file.

## Acknowledgments

- FFmpeg for transcoding
- modernc.org/sqlite for pure-Go SQLite
- google/uuid for session management
