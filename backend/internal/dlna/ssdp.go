package dlna

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	ssdpMulticastAddr = "239.255.255.250:1900"
	ssdpMaxAge        = 1800 // Seconds
)

// SSDPServer handles SSDP discovery protocol
type SSDPServer struct {
	uuid         string
	friendlyName string
	serverAddr   string // e.g., "http://192.168.1.100:8080"

	conn     *net.UDPConn
	mu       sync.RWMutex
	devices  map[string]*DLNADevice
	running  bool
	stopChan chan struct{}
}

// DLNADevice represents a discovered DLNA device (renderer)
type DLNADevice struct {
	UUID         string    `json:"uuid"`
	FriendlyName string    `json:"friendly_name"`
	Location     string    `json:"location"`
	DeviceType   string    `json:"device_type"`
	LastSeen     time.Time `json:"last_seen"`
	Manufacturer string    `json:"manufacturer,omitempty"`
	ModelName    string    `json:"model_name,omitempty"`
}

// NewSSDPServer creates a new SSDP server
func NewSSDPServer(deviceUUID, friendlyName, serverAddr string) *SSDPServer {
	if deviceUUID == "" {
		deviceUUID = uuid.New().String()
	}

	return &SSDPServer{
		uuid:         deviceUUID,
		friendlyName: friendlyName,
		serverAddr:   serverAddr,
		devices:      make(map[string]*DLNADevice),
		stopChan:     make(chan struct{}),
	}
}

// Start starts the SSDP server
func (s *SSDPServer) Start(ctx context.Context) error {
	// Resolve multicast address
	multicastAddr, err := net.ResolveUDPAddr("udp4", ssdpMulticastAddr)
	if err != nil {
		return fmt.Errorf("failed to resolve multicast address: %w", err)
	}

	// Create UDP connection for multicast
	conn, err := net.ListenMulticastUDP("udp4", nil, multicastAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on multicast: %w", err)
	}

	s.conn = conn
	s.running = true

	// Start listener goroutine
	go s.listen(ctx)

	// Start advertiser goroutine
	go s.advertise(ctx)

	// Start device cleanup goroutine
	go s.cleanupDevices(ctx)

	// Discovery existing devices
	s.searchForDevices()

	return nil
}

// Stop stops the SSDP server
func (s *SSDPServer) Stop() {
	s.running = false
	close(s.stopChan)
	if s.conn != nil {
		s.conn.Close()
	}
}

// listen handles incoming SSDP messages
func (s *SSDPServer) listen(ctx context.Context) {
	buffer := make([]byte, 4096)

	for s.running {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		default:
		}

		s.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, remoteAddr, err := s.conn.ReadFromUDP(buffer)
		if err != nil {
			if !isTimeout(err) {
				log.Printf("[SSDP] Read error: %v", err)
			}
			continue
		}

		message := string(buffer[:n])
		s.handleMessage(message, remoteAddr)
	}
}

// handleMessage processes an incoming SSDP message
func (s *SSDPServer) handleMessage(message string, remoteAddr *net.UDPAddr) {
	lines := strings.Split(message, "\r\n")
	if len(lines) == 0 {
		return
	}

	headers := parseHeaders(lines[1:])

	// Handle M-SEARCH requests
	if strings.HasPrefix(lines[0], "M-SEARCH") {
		s.handleSearch(headers, remoteAddr)
		return
	}

	// Handle NOTIFY messages (device announcements)
	if strings.HasPrefix(lines[0], "NOTIFY") {
		s.handleNotify(headers)
		return
	}

	// Handle HTTP responses (search responses)
	if strings.HasPrefix(lines[0], "HTTP/1.1 200") {
		s.handleSearchResponse(headers)
		return
	}
}

// handleSearch responds to M-SEARCH requests
func (s *SSDPServer) handleSearch(headers map[string]string, remoteAddr *net.UDPAddr) {
	st := headers["ST"]
	if st == "" {
		st = headers["st"]
	}

	// Check if we should respond to this search
	shouldRespond := false
	switch st {
	case "ssdp:all", "upnp:rootdevice":
		shouldRespond = true
	case "urn:schemas-upnp-org:device:MediaServer:1":
		shouldRespond = true
	case "urn:schemas-upnp-org:service:ContentDirectory:1":
		shouldRespond = true
	case "urn:schemas-upnp-org:service:ConnectionManager:1":
		shouldRespond = true
	}

	if shouldRespond {
		s.sendSearchResponse(remoteAddr, st)
	}
}

// sendSearchResponse sends a response to an M-SEARCH request
func (s *SSDPServer) sendSearchResponse(addr *net.UDPAddr, st string) {
	response := fmt.Sprintf(
		"HTTP/1.1 200 OK\r\n"+
			"CACHE-CONTROL: max-age=%d\r\n"+
			"DATE: %s\r\n"+
			"EXT:\r\n"+
			"LOCATION: %s/dlna/device.xml\r\n"+
			"SERVER: Linux/5.10 UPnP/1.0 DLNATranscoder/1.0\r\n"+
			"ST: %s\r\n"+
			"USN: uuid:%s::upnp:rootdevice\r\n"+
			"\r\n",
		ssdpMaxAge,
		time.Now().UTC().Format(time.RFC1123),
		s.serverAddr,
		st,
		s.uuid,
	)

	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		log.Printf("[SSDP] Failed to dial response: %v", err)
		return
	}
	defer conn.Close()

	conn.Write([]byte(response))
}

// handleNotify processes NOTIFY announcements from other devices
func (s *SSDPServer) handleNotify(headers map[string]string) {
	nts := headers["NTS"]
	if nts == "" {
		nts = headers["nts"]
	}

	usn := headers["USN"]
	if usn == "" {
		usn = headers["usn"]
	}

	location := headers["LOCATION"]
	if location == "" {
		location = headers["location"]
	}

	nt := headers["NT"]
	if nt == "" {
		nt = headers["nt"]
	}

	// We're interested in media renderers and any device with render/TV capability
	lowerNT := strings.ToLower(nt)
	lowerLoc := strings.ToLower(location)
	isRenderer := strings.Contains(lowerNT, "mediarenderer") ||
		strings.Contains(lowerNT, "avtransport") ||
		strings.Contains(lowerNT, "renderingcontrol") ||
		strings.Contains(lowerNT, "tvrenderer") ||
		strings.Contains(lowerLoc, "render") ||
		strings.Contains(lowerLoc, "tv") ||
		strings.Contains(lowerLoc, "display")

	if !isRenderer {
		return
	}

	uuid := extractUUID(usn)
	if uuid == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch nts {
	case "ssdp:alive":
		if _, exists := s.devices[uuid]; !exists {
			device := &DLNADevice{
				UUID:       uuid,
				DeviceType: nt,
				Location:   location,
				LastSeen:   time.Now(),
			}
			s.devices[uuid] = device
			log.Printf("[SSDP] Discovered device: %s at %s", uuid, location)

			// Fetch device details asynchronously
			go s.fetchDeviceDetails(uuid, location)
		} else {
			s.devices[uuid].LastSeen = time.Now()
		}

	case "ssdp:byebye":
		delete(s.devices, uuid)
	}
}

// handleSearchResponse processes responses to our M-SEARCH requests
func (s *SSDPServer) handleSearchResponse(headers map[string]string) {
	location := headers["LOCATION"]
	if location == "" {
		location = headers["location"]
	}

	usn := headers["USN"]
	if usn == "" {
		usn = headers["usn"]
	}

	st := headers["ST"]
	if st == "" {
		st = headers["st"]
	}

	// We're interested in media renderers and any device with render/TV capability
	lowerST := strings.ToLower(st)
	lowerLoc := strings.ToLower(location)
	isRenderer := strings.Contains(lowerST, "mediarenderer") ||
		strings.Contains(lowerST, "avtransport") ||
		strings.Contains(lowerST, "renderingcontrol") ||
		strings.Contains(lowerST, "tvrenderer") ||
		strings.Contains(lowerLoc, "render") ||
		strings.Contains(lowerLoc, "tv") ||
		strings.Contains(lowerLoc, "display") ||
		st == "ssdp:all" // Accept responses to ssdp:all that look like TVs

	if !isRenderer {
		return
	}

	uuid := extractUUID(usn)
	if uuid == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.devices[uuid]; !exists {
		device := &DLNADevice{
			UUID:       uuid,
			DeviceType: st,
			Location:   location,
			LastSeen:   time.Now(),
		}
		s.devices[uuid] = device
		log.Printf("[SSDP] Discovered device via search: %s at %s", uuid, location)

		// Fetch device details asynchronously
		go s.fetchDeviceDetails(uuid, location)
	} else {
		s.devices[uuid].LastSeen = time.Now()
	}
}

// advertise periodically broadcasts SSDP NOTIFY messages
func (s *SSDPServer) advertise(ctx context.Context) {
	// Initial advertisement
	s.sendNotify("ssdp:alive")

	ticker := time.NewTicker(time.Duration(ssdpMaxAge/2) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.sendNotify("ssdp:byebye")
			return
		case <-s.stopChan:
			s.sendNotify("ssdp:byebye")
			return
		case <-ticker.C:
			s.sendNotify("ssdp:alive")
		}
	}
}

// sendNotify sends a SSDP NOTIFY message
func (s *SSDPServer) sendNotify(nts string) {
	multicastAddr, _ := net.ResolveUDPAddr("udp4", ssdpMulticastAddr)
	conn, err := net.DialUDP("udp4", nil, multicastAddr)
	if err != nil {
		log.Printf("[SSDP] Failed to dial notify: %v", err)
		return
	}
	defer conn.Close()

	// Send notifications for different device types
	types := []string{
		"upnp:rootdevice",
		"urn:schemas-upnp-org:device:MediaServer:1",
		"urn:schemas-upnp-org:service:ContentDirectory:1",
		"urn:schemas-upnp-org:service:ConnectionManager:1",
	}

	for _, nt := range types {
		message := fmt.Sprintf(
			"NOTIFY * HTTP/1.1\r\n"+
				"HOST: %s\r\n"+
				"CACHE-CONTROL: max-age=%d\r\n"+
				"LOCATION: %s/dlna/device.xml\r\n"+
				"NT: %s\r\n"+
				"NTS: %s\r\n"+
				"SERVER: Linux/5.10 UPnP/1.0 DLNATranscoder/1.0\r\n"+
				"USN: uuid:%s::%s\r\n"+
				"\r\n",
			ssdpMulticastAddr,
			ssdpMaxAge,
			s.serverAddr,
			nt,
			nts,
			s.uuid,
			nt,
		)

		conn.Write([]byte(message))
		time.Sleep(50 * time.Millisecond) // Small delay between messages
	}
}

// searchForDevices sends M-SEARCH requests to discover DLNA renderers
func (s *SSDPServer) searchForDevices() {
	multicastAddr, _ := net.ResolveUDPAddr("udp4", ssdpMulticastAddr)
	conn, err := net.DialUDP("udp4", nil, multicastAddr)
	if err != nil {
		log.Printf("[SSDP] Failed to dial M-SEARCH: %v", err)
		return
	}
	defer conn.Close()

	// Search for media renderers and all devices
	targets := []string{
		"ssdp:all",
		"urn:schemas-upnp-org:device:MediaRenderer:1",
		"urn:schemas-upnp-org:service:AVTransport:1",
		"urn:schemas-upnp-org:service:RenderingControl:1",
	}

	for _, st := range targets {
		message := fmt.Sprintf(
			"M-SEARCH * HTTP/1.1\r\n"+
				"HOST: %s\r\n"+
				"MAN: \"ssdp:discover\"\r\n"+
				"MX: 3\r\n"+
				"ST: %s\r\n"+
				"\r\n",
			ssdpMulticastAddr,
			st,
		)

		conn.Write([]byte(message))
		time.Sleep(50 * time.Millisecond)
	}
}

// fetchDeviceDetails retrieves detailed information about a device
func (s *SSDPServer) fetchDeviceDetails(uuid, location string) {
	// This would typically fetch the device XML and parse it
	// For now, we'll just update what we know from SSDP
	// Full implementation would use HTTP to fetch location URL
}

// cleanupDevices removes stale devices
func (s *SSDPServer) cleanupDevices(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.mu.Lock()
			cutoff := time.Now().Add(-time.Duration(ssdpMaxAge*2) * time.Second)
			for uuid, device := range s.devices {
				if device.LastSeen.Before(cutoff) {
					delete(s.devices, uuid)
					// Silently remove stale devices
				}
			}
			s.mu.Unlock()
		}
	}
}

// GetDevices returns all discovered DLNA devices
func (s *SSDPServer) GetDevices() []*DLNADevice {
	s.mu.RLock()
	defer s.mu.RUnlock()

	devices := make([]*DLNADevice, 0, len(s.devices))
	for _, device := range s.devices {
		devices = append(devices, device)
	}
	return devices
}

// GetDevice returns a specific device by UUID
func (s *SSDPServer) GetDevice(uuid string) (*DLNADevice, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	device, ok := s.devices[uuid]
	if !ok {
		return nil, fmt.Errorf("device not found: %s", uuid)
	}
	return device, nil
}

// GetUUID returns the server's UUID
func (s *SSDPServer) GetUUID() string {
	return s.uuid
}

// RefreshDevices triggers a new device search
func (s *SSDPServer) RefreshDevices() {
	s.searchForDevices()
}

// AddManualDevice manually adds a device by IP address
func (s *SSDPServer) AddManualDevice(ip, port, path, friendlyName string) {
	location := fmt.Sprintf("http://%s:%s%s", ip, port, path)
	uuid := fmt.Sprintf("manual-%s-%s", ip, port)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Don't add if already exists
	if _, exists := s.devices[uuid]; exists {
		return
	}

	device := &DLNADevice{
		UUID:         uuid,
		FriendlyName: friendlyName,
		Location:     location,
		DeviceType:   "Manual DLNA Renderer",
		LastSeen:     time.Now(),
	}
	s.devices[uuid] = device
	log.Printf("[SSDP] Added manual device: %s at %s", friendlyName, location)
}

// Helper functions

func parseHeaders(lines []string) map[string]string {
	headers := make(map[string]string)
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			headers[key] = value
		}
	}
	return headers
}

func extractUUID(usn string) string {
	// USN format: uuid:XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX::...
	parts := strings.Split(usn, "::")
	if len(parts) > 0 {
		uuidPart := strings.TrimPrefix(parts[0], "uuid:")
		return uuidPart
	}
	return ""
}

func isTimeout(err error) bool {
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	return false
}
