package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wysentanu/dlna-movie-cast/internal/api"
	"github.com/wysentanu/dlna-movie-cast/internal/config"
	"github.com/wysentanu/dlna-movie-cast/internal/dlna"
	"github.com/wysentanu/dlna-movie-cast/internal/library"
)

func main() {
	log.Println("Starting DLNA Transcoder Media Server...")

	// Load configuration
	cfg := config.DefaultConfig()
	cfg.LoadFromEnv()

	// Ensure data directories exist
	if err := cfg.EnsureDirectories(); err != nil {
		log.Fatalf("Failed to create directories: %v", err)
	}

	// Get server IP address
	serverIP := getLocalIP()
	serverAddr := fmt.Sprintf("http://%s:%d", serverIP, cfg.ServerPort)
	log.Printf("Server address: %s", serverAddr)

	// Initialize library
	lib, err := library.NewLibrary(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize library: %v", err)
	}
	defer lib.Close()

	// Initial library scan
	log.Println("Scanning media library...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	if err := lib.Scan(ctx); err != nil {
		log.Printf("Warning: Library scan error: %v", err)
	}
	cancel()
	log.Printf("Found %d movies", len(lib.GetAllMovies()))

	// Initialize SSDP server
	ssdp := dlna.NewSSDPServer(cfg.DLNAUUID, cfg.DLNAFriendlyName, serverAddr)

	// Start SSDP
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	if err := ssdp.Start(ctx); err != nil {
		log.Fatalf("Failed to start SSDP server: %v", err)
	}
	log.Println("SSDP server started")

	// Initialize API
	apiHandler, err := api.NewAPI(cfg, lib, ssdp, serverAddr)
	if err != nil {
		log.Fatalf("Failed to initialize API: %v", err)
	}

	// Setup HTTP routes
	mux := http.NewServeMux()
	apiHandler.SetupRoutes(mux)

	// Serve static files for frontend (if present)
	fs := http.FileServer(http.Dir("../frontend/dist"))
	mux.Handle("/", http.StripPrefix("/", fs))

	// Start HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // No write timeout for streaming
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("HTTP server listening on %s:%d", cfg.ServerHost, cfg.ServerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	ssdp.Stop()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}

// getLocalIP returns the local IP address
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}

	return "127.0.0.1"
}
