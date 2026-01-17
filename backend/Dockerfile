# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git build-base

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o /server ./cmd/server/main.go

# Runtime stage - Use Debian for Rockchip support
FROM debian:bookworm-slim

WORKDIR /app

# Install base dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Install Jellyfin FFmpeg 7 deb package (has rkmpp support for Rockchip)
RUN curl -L -o /tmp/jellyfin-ffmpeg.deb \
    "https://github.com/jellyfin/jellyfin-ffmpeg/releases/download/v7.1.3-1/jellyfin-ffmpeg7_7.1.3-1-bookworm_arm64.deb" \
    && apt-get update \
    && apt-get install -y --no-install-recommends /tmp/jellyfin-ffmpeg.deb \
    && rm /tmp/jellyfin-ffmpeg.deb \
    && rm -rf /var/lib/apt/lists/* \
    && ln -sf /usr/lib/jellyfin-ffmpeg/ffmpeg /usr/bin/ffmpeg \
    && ln -sf /usr/lib/jellyfin-ffmpeg/ffprobe /usr/bin/ffprobe

# Create necessary directories
RUN mkdir -p /media /data /tmp/dlna-movie-cast-hls

# Copy binary
COPY --from=builder /server .

# Environment Defaults
ENV MEDIA_PATH=/media \
    DB_PATH=/data/library.db \
    SERVER_PORT=8080

EXPOSE 8080

CMD ["./server"]
