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

# Runtime stage - Use Debian for better Rockchip support
FROM debian:bookworm-slim

WORKDIR /app

# Install base dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    xz-utils \
    && rm -rf /var/lib/apt/lists/*

# Install Jellyfin FFmpeg 7 portable (has rkmpp support for Rockchip)
RUN curl -L -o /tmp/ffmpeg.tar.xz \
    "https://github.com/jellyfin/jellyfin-ffmpeg/releases/download/v7.1.3-1/jellyfin-ffmpeg_7.1.3-1_portable_linuxarm64-gpl.tar.xz" \
    && mkdir -p /opt/jellyfin-ffmpeg \
    && tar -xf /tmp/ffmpeg.tar.xz -C /opt/jellyfin-ffmpeg --strip-components=1 \
    && ln -s /opt/jellyfin-ffmpeg/ffmpeg /usr/bin/ffmpeg \
    && ln -s /opt/jellyfin-ffmpeg/ffprobe /usr/bin/ffprobe \
    && rm /tmp/ffmpeg.tar.xz

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
