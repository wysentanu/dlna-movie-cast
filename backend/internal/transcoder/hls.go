package transcoder

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// HLSSession represents an active HLS transcoding session
type HLSSession struct {
	ID           string
	MovieID      string
	Dir          string
	LastAccessed time.Time
	Process      *os.Process
}

// HLSManager manages HLS sessions
type HLSManager struct {
	sessions map[string]*HLSSession
	mu       sync.RWMutex
	baseDir  string
	stopChan chan struct{}
}

// NewHLSManager creates a new HLS manager
func NewHLSManager(baseDir string) (*HLSManager, error) {
	// Clean up any leftover sessions from previous runs
	_ = os.RemoveAll(baseDir)

	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, err
	}

	m := &HLSManager{
		sessions: make(map[string]*HLSSession),
		baseDir:  baseDir,
		stopChan: make(chan struct{}),
	}

	// Start cleanup routine
	go m.cleanupRoutine()

	return m, nil
}

// GetSession returns an existing session for a movie or nil
func (m *HLSManager) GetSession(movieID string) *HLSSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, s := range m.sessions {
		if s.MovieID == movieID {
			// Update access time
			s.LastAccessed = time.Now()
			return s
		}
	}
	return nil
}

// CreateSession creates a new HLS session
func (m *HLSManager) CreateSession(movieID string) (*HLSSession, error) {
	// Check if session already exists
	if s := m.GetSession(movieID); s != nil {
		return s, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double check
	for _, s := range m.sessions {
		if s.MovieID == movieID {
			return s, nil
		}
	}

	id := uuid.New().String()
	dir := filepath.Join(m.baseDir, id)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	session := &HLSSession{
		ID:           id,
		MovieID:      movieID,
		Dir:          dir,
		LastAccessed: time.Now(),
	}

	m.sessions[id] = session
	return session, nil
}

// GetSessionByID returns a session by its ID
func (m *HLSManager) GetSessionByID(id string) *HLSSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if s, ok := m.sessions[id]; ok {
		s.LastAccessed = time.Now()
		return s
	}
	return nil
}

// Stop stops the manager and cleans up
func (m *HLSManager) Stop() {
	close(m.stopChan)
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, s := range m.sessions {
		m.terminateSession(s)
	}
}

// cleanupRoutine periodically removes old sessions
func (m *HLSManager) cleanupRoutine() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.cleanupOldSessions()
		}
	}
}

func (m *HLSManager) cleanupOldSessions() {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-10 * time.Minute) // 10 minute timeout

	for id, s := range m.sessions {
		if s.LastAccessed.Before(cutoff) {
			m.terminateSession(s)
			delete(m.sessions, id)
			log.Printf("HLS: Cleaned up expired session %s for movie %s", id, s.MovieID)
		}
	}
}

func (m *HLSManager) terminateSession(s *HLSSession) {
	if s.Process != nil {
		s.Process.Kill()
	}
	os.RemoveAll(s.Dir)
}

// GetPlaylistPath returns the path to the playlist file
func (m *HLSManager) GetPlaylistPath(sessionID string) string {
	return filepath.Join(m.baseDir, sessionID, "playlist.m3u8")
}

// CopySegment copies a segment file to the writer
func (m *HLSManager) CopySegment(sessionID, segmentName string, w io.Writer) error {
	// Security: validate segment name to prevent directory traversal
	cleanName := filepath.Clean(segmentName)
	if cleanName != segmentName || filepath.IsAbs(segmentName) || cleanName == ".." {
		return os.ErrPermission
	}

	path := filepath.Join(m.baseDir, sessionID, cleanName)

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(w, f)
	return err
}
