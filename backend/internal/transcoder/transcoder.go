package transcoder

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wysentanu/dlna-movie-cast/internal/config"
	"github.com/wysentanu/dlna-movie-cast/internal/library"
)

// TranscodeOptions specifies transcoding parameters
type TranscodeOptions struct {
	// Video settings
	VideoCodec   string // Target video codec (h264, hevc)
	VideoBitrate string // Video bitrate (e.g., "8M")
	Width        int    // Target width (0 = auto)
	Height       int    // Target height (0 = auto)

	// Audio settings
	AudioCodec   string // Target audio codec (aac, mp3)
	AudioBitrate string // Audio bitrate (e.g., "192k")

	// Subtitle
	SubtitlePath  string // Path to external subtitle file to burn
	SubtitleIndex int    // Index of embedded subtitle track (-1 = none)

	// Seeking
	StartTime int // Start time in seconds
	Duration  int // Duration in seconds (0 = until end)

	// Hardware acceleration
	UseHardwareAccel bool // Use hardware encoder if available

	// Output
	Format     string // "mp4" or "hls"
	OutputPath string // Output directory for HLS
}

// DefaultOptions returns default transcoding options
func DefaultOptions(cfg *config.Config) TranscodeOptions {
	return TranscodeOptions{
		VideoCodec:       "h264",
		VideoBitrate:     cfg.VideoBitrate,
		AudioCodec:       "aac",
		AudioBitrate:     cfg.AudioBitrate,
		SubtitleIndex:    -1,
		UseHardwareAccel: isHardwareAccelAvailable(),
	}
}

// isHardwareAccelAvailable checks if hardware acceleration is available
func isHardwareAccelAvailable() bool {
	// Currently checks for Rockchip MPP (/dev/mpp_service)
	// Can be extended to support VAAPI, NVENC, etc.
	cmd := exec.Command("test", "-e", "/dev/mpp_service")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

// Transcoder handles video transcoding with FFmpeg
type Transcoder struct {
	config *config.Config
}

// NewTranscoder creates a new transcoder instance
func NewTranscoder(cfg *config.Config) *Transcoder {
	return &Transcoder{
		config: cfg,
	}
}

// Transcode starts transcoding a movie and returns a reader for the output
func (t *Transcoder) Transcode(ctx context.Context, movie *library.Movie, opts TranscodeOptions) (io.ReadCloser, error) {
	args := t.buildFFmpegArgs(movie, opts)

	cmd := exec.CommandContext(ctx, t.config.FFmpegPath, args...)

	// Capture stderr for error logging
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Drain stderr to prevent blocking
	go func() {
		io.Copy(io.Discard, stderr)
	}()

	// Wrap the stdout in a custom reader that waits for the process to exit
	return &transcodeReader{
		ReadCloser: stdout,
		cmd:        cmd,
	}, nil
}

// buildFFmpegArgs constructs the FFmpeg command arguments
func (t *Transcoder) buildFFmpegArgs(movie *library.Movie, opts TranscodeOptions) []string {
	var args []string

	// Global options
	args = append(args, "-hide_banner", "-loglevel", "warning")

	// Hardware acceleration for decoding (if available)
	if opts.UseHardwareAccel {
		args = append(args,
			"-hwaccel", "rkmpp",
			"-hwaccel_output_format", "drm_prime",
		)
	}

	// Seeking (before input for faster seeking)
	if opts.StartTime > 0 {
		args = append(args, "-ss", strconv.Itoa(opts.StartTime))
	}

	// Input file
	args = append(args, "-i", movie.FilePath)

	// Duration
	if opts.Duration > 0 {
		args = append(args, "-t", strconv.Itoa(opts.Duration))
	}

	// Build video filter chain
	var videoFilters []string

	if opts.UseHardwareAccel {
		// Download from GPU for subtitle processing
		videoFilters = append(videoFilters, "hwdownload", "format=nv12")
	}

	// Subtitle burning
	if opts.SubtitlePath != "" {
		// Escape the subtitle path for FFmpeg filter syntax
		escapedPath := strings.ReplaceAll(opts.SubtitlePath, ":", "\\:")
		escapedPath = strings.ReplaceAll(escapedPath, "'", "\\'")
		escapedPath = strings.ReplaceAll(escapedPath, "[", "\\[")
		escapedPath = strings.ReplaceAll(escapedPath, "]", "\\]")
		videoFilters = append(videoFilters, fmt.Sprintf("subtitles='%s'", escapedPath))
	} else if opts.SubtitleIndex >= 0 {
		// Burn embedded subtitle
		videoFilters = append(videoFilters, fmt.Sprintf("subtitles='%s':si=%d",
			strings.ReplaceAll(movie.FilePath, "'", "\\'"), opts.SubtitleIndex))
	}

	// Scaling
	if opts.Width > 0 || opts.Height > 0 {
		w := opts.Width
		h := opts.Height
		if w == 0 {
			w = -2 // Maintain aspect ratio
		}
		if h == 0 {
			h = -2
		}
		videoFilters = append(videoFilters, fmt.Sprintf("scale=%d:%d", w, h))
	}

	// Upload back to GPU for hardware encoding
	if opts.UseHardwareAccel {
		videoFilters = append(videoFilters, "format=nv12", "hwupload")
	}

	// Apply video filter chain
	if len(videoFilters) > 0 {
		args = append(args, "-vf", strings.Join(videoFilters, ","))
	}

	// Video codec settings
	if opts.UseHardwareAccel {
		// Use hardware encoder (Rockchip MPP)
		switch opts.VideoCodec {
		case "hevc", "h265":
			args = append(args, "-c:v", "hevc_rkmpp")
		default:
			args = append(args, "-c:v", "h264_rkmpp")
		}
	} else {
		// Software encoding (default)
		switch opts.VideoCodec {
		case "hevc", "h265":
			args = append(args, "-c:v", "libx265", "-pix_fmt", "yuv420p")
		default:
			args = append(args,
				"-c:v", "libx264",
				"-preset", t.config.Preset,
				"-pix_fmt", "yuv420p",
				"-profile:v", "high",
				"-level:v", "4.0",
				"-colorspace", "bt709",
				"-color_primaries", "bt709",
				"-color_trc", "bt709",
				"-color_range", "tv",
			)
		}
	}

	// Video bitrate
	args = append(args, "-b:v", opts.VideoBitrate)

	// Audio codec settings
	args = append(args, "-c:a", opts.AudioCodec, "-b:a", opts.AudioBitrate)

	// Output format
	if opts.Format == "hls" {
		// HLS specific options
		segmentFilename := filepath.Join(opts.OutputPath, "segment_%03d.ts")
		playlistFilename := filepath.Join(opts.OutputPath, "playlist.m3u8")

		// Force keyframe at segment boundaries for clean cuts
		// GOP size = framerate * segment_time (assume 30fps * 10s = 300)
		args = append(args,
			"-g", "300", // GOP size matching segment length
			"-keyint_min", "300", // Minimum keyframe interval
			"-sc_threshold", "0", // Disable scene change detection for consistent segments
		)

		args = append(args,
			"-f", "hls",
			"-hls_time", "10", // 10 second segments for better buffering
			"-hls_list_size", "0", // Keep all segments in playlist
			"-hls_segment_filename", segmentFilename,
			"-hls_flags", "independent_segments", // Each segment is independently decodable
			"-hls_playlist_type", "event", // Growing playlist (not VOD)
			"-start_number", "0",
			playlistFilename,
		)
	} else {
		// Output format for direct streaming (MP4)
		args = append(args,
			"-movflags", "frag_keyframe+empty_moov+faststart",
			"-f", "mp4",
			"pipe:1", // Output to stdout
		)
	}

	return args
}

// StartHLSTranscode starts an HLS transcoding session
func (t *Transcoder) StartHLSTranscode(ctx context.Context, movie *library.Movie, opts TranscodeOptions) (*os.Process, error) {
	opts.Format = "hls"
	args := t.buildFFmpegArgs(movie, opts)

	cmd := exec.CommandContext(ctx, t.config.FFmpegPath, args...)

	// Capture stderr for error logging
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Drain stderr and wait for process to finish
	go func() {
		io.Copy(io.Discard, stderr)
		cmd.Wait()
	}()

	return cmd.Process, nil
}

// GetDirectStreamURL returns a URL for direct streaming (no transcode)
func (t *Transcoder) NeedsTranscode(movie *library.Movie, burnSubtitle bool) bool {
	// Check if video codec is compatible with most DLNA devices
	compatibleCodecs := []string{"h264", "avc", "avc1"}
	codecCompatible := false
	for _, codec := range compatibleCodecs {
		if strings.EqualFold(movie.VideoCodec, codec) {
			codecCompatible = true
			break
		}
	}

	// If subtitle burning is requested, always transcode
	if burnSubtitle {
		return true
	}

	// If codec is not compatible, transcode
	return !codecCompatible
}

// transcodeReader wraps the FFmpeg stdout and ensures cleanup
type transcodeReader struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (r *transcodeReader) Close() error {
	err := r.ReadCloser.Close()

	// Kill the process if still running
	if r.cmd.Process != nil {
		r.cmd.Process.Kill()
	}

	// Wait for the process to exit
	r.cmd.Wait()

	return err
}
