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
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /server ./cmd/server/main.go

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies (FFmpeg, etc)
RUN apk add --no-cache \
    ffmpeg \
    ca-certificates \
    tzdata

# Note: For Intel/AMD hardware acceleration, you may need to add:
# libva-intel-driver mesa-va-gallium

# Create necessary directories
RUN mkdir -p /media /data /tmp/dlna-movie-cast-hls

# Copy binary
COPY --from=builder /server .

# Environment Defaults
ENV MEDIA_PATH=/media \
    DB_PATH=/data/library.db \
    SERVER_PORT=8080 \
    GIN_MODE=release

EXPOSE 8080

CMD ["./server"]
