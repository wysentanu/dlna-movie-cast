package dlna

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AVTransportController controls playback on DLNA renderers
type AVTransportController struct {
	httpClient *http.Client
}

// NewAVTransportController creates a new AVTransport controller
func NewAVTransportController() *AVTransportController {
	return &AVTransportController{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// PlaybackState represents the current playback state
type PlaybackState struct {
	TransportState  string `json:"transport_state"`
	CurrentPosition string `json:"current_position"`
	Duration        string `json:"duration"`
	CurrentURI      string `json:"current_uri"`
}

// SetAVTransportURI sets the media URL on the renderer
func (c *AVTransportController) SetAVTransportURI(device *DLNADevice, mediaURL, title string) error {
	controlURL := c.getAVTransportControlURL(device)
	if controlURL == "" {
		return fmt.Errorf("AVTransport control URL not found for device")
	}

	// Build DIDL-Lite metadata
	metadata := fmt.Sprintf(`&lt;DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"&gt;&lt;item id="0" parentID="-1" restricted="1"&gt;&lt;dc:title&gt;%s&lt;/dc:title&gt;&lt;res protocolInfo="http-get:*:video/mp4:*"&gt;%s&lt;/res&gt;&lt;upnp:class&gt;object.item.videoItem&lt;/upnp:class&gt;&lt;/item&gt;&lt;/DIDL-Lite&gt;`,
		xmlEscape(title),
		xmlEscape(mediaURL),
	)

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:SetAVTransportURI xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
<InstanceID>0</InstanceID>
<CurrentURI>%s</CurrentURI>
<CurrentURIMetaData>%s</CurrentURIMetaData>
</u:SetAVTransportURI>
</s:Body>
</s:Envelope>`, xmlEscape(mediaURL), metadata)

	return c.sendSOAPAction(controlURL, "urn:schemas-upnp-org:service:AVTransport:1#SetAVTransportURI", body)
}

// Play starts playback on the renderer
func (c *AVTransportController) Play(device *DLNADevice) error {
	controlURL := c.getAVTransportControlURL(device)
	if controlURL == "" {
		return fmt.Errorf("AVTransport control URL not found for device")
	}

	body := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:Play xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
<InstanceID>0</InstanceID>
<Speed>1</Speed>
</u:Play>
</s:Body>
</s:Envelope>`

	return c.sendSOAPAction(controlURL, "urn:schemas-upnp-org:service:AVTransport:1#Play", body)
}

// Pause pauses playback on the renderer
func (c *AVTransportController) Pause(device *DLNADevice) error {
	controlURL := c.getAVTransportControlURL(device)
	if controlURL == "" {
		return fmt.Errorf("AVTransport control URL not found for device")
	}

	body := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:Pause xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
<InstanceID>0</InstanceID>
</u:Pause>
</s:Body>
</s:Envelope>`

	return c.sendSOAPAction(controlURL, "urn:schemas-upnp-org:service:AVTransport:1#Pause", body)
}

// Stop stops playback on the renderer
func (c *AVTransportController) Stop(device *DLNADevice) error {
	controlURL := c.getAVTransportControlURL(device)
	if controlURL == "" {
		return fmt.Errorf("AVTransport control URL not found for device")
	}

	body := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:Stop xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
<InstanceID>0</InstanceID>
</u:Stop>
</s:Body>
</s:Envelope>`

	return c.sendSOAPAction(controlURL, "urn:schemas-upnp-org:service:AVTransport:1#Stop", body)
}

// Seek seeks to a specific position (format: HH:MM:SS)
func (c *AVTransportController) Seek(device *DLNADevice, position string) error {
	controlURL := c.getAVTransportControlURL(device)
	if controlURL == "" {
		return fmt.Errorf("AVTransport control URL not found for device")
	}

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:Seek xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
<InstanceID>0</InstanceID>
<Unit>REL_TIME</Unit>
<Target>%s</Target>
</u:Seek>
</s:Body>
</s:Envelope>`, position)

	return c.sendSOAPAction(controlURL, "urn:schemas-upnp-org:service:AVTransport:1#Seek", body)
}

// GetTransportInfo gets the current transport state
func (c *AVTransportController) GetTransportInfo(device *DLNADevice) (*PlaybackState, error) {
	controlURL := c.getAVTransportControlURL(device)
	if controlURL == "" {
		return nil, fmt.Errorf("AVTransport control URL not found for device")
	}

	body := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:GetTransportInfo xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
<InstanceID>0</InstanceID>
</u:GetTransportInfo>
</s:Body>
</s:Envelope>`

	resp, err := c.sendSOAPActionWithResponse(controlURL, "urn:schemas-upnp-org:service:AVTransport:1#GetTransportInfo", body)
	if err != nil {
		return nil, err
	}

	state := &PlaybackState{
		TransportState: extractXMLValue(resp, "CurrentTransportState"),
	}

	return state, nil
}

// GetPositionInfo gets the current playback position
func (c *AVTransportController) GetPositionInfo(device *DLNADevice) (*PlaybackState, error) {
	controlURL := c.getAVTransportControlURL(device)
	if controlURL == "" {
		return nil, fmt.Errorf("AVTransport control URL not found for device")
	}

	body := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:GetPositionInfo xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
<InstanceID>0</InstanceID>
</u:GetPositionInfo>
</s:Body>
</s:Envelope>`

	resp, err := c.sendSOAPActionWithResponse(controlURL, "urn:schemas-upnp-org:service:AVTransport:1#GetPositionInfo", body)
	if err != nil {
		return nil, err
	}

	state := &PlaybackState{
		CurrentPosition: extractXMLValue(resp, "RelTime"),
		Duration:        extractXMLValue(resp, "TrackDuration"),
		CurrentURI:      extractXMLValue(resp, "TrackURI"),
	}

	return state, nil
}

// sendSOAPAction sends a SOAP action request
func (c *AVTransportController) sendSOAPAction(controlURL, soapAction, body string) error {
	_, err := c.sendSOAPActionWithResponse(controlURL, soapAction, body)
	return err
}

// sendSOAPActionWithResponse sends a SOAP action and returns the response
func (c *AVTransportController) sendSOAPActionWithResponse(controlURL, soapAction, body string) (string, error) {
	req, err := http.NewRequest("POST", controlURL, bytes.NewBufferString(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", fmt.Sprintf(`"%s"`, soapAction))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("SOAP action failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return string(respBody), nil
}

// getAVTransportControlURL extracts the AVTransport control URL from the device
func (c *AVTransportController) getAVTransportControlURL(device *DLNADevice) string {
	if device.Location == "" {
		return ""
	}

	// Fetch device description
	resp, err := c.httpClient.Get(device.Location)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	// Parse XML to find AVTransport control URL
	controlURL := c.extractControlURL(string(body), "AVTransport")
	if controlURL == "" {
		return ""
	}

	// Make the URL absolute if it's relative
	baseURL := device.Location
	if idx := strings.LastIndex(baseURL, "/"); idx > 0 {
		baseURL = baseURL[:idx]
	}

	if strings.HasPrefix(controlURL, "/") {
		// Relative to host root - extract host from location
		parts := strings.SplitN(device.Location, "/", 4)
		if len(parts) >= 3 {
			return parts[0] + "//" + parts[2] + controlURL
		}
	}

	if !strings.HasPrefix(controlURL, "http") {
		return baseURL + "/" + controlURL
	}

	return controlURL
}

// extractControlURL parses the device description XML to find a service control URL
func (c *AVTransportController) extractControlURL(xmlContent, serviceType string) string {
	// Simple parsing - look for the service containing the serviceType
	// and extract its controlURL

	// Find the service block containing the serviceType
	serviceStart := strings.Index(xmlContent, serviceType)
	if serviceStart == -1 {
		return ""
	}

	// Look for controlURL within this service block
	// The service block ends at </service>
	serviceEnd := strings.Index(xmlContent[serviceStart:], "</service>")
	if serviceEnd == -1 {
		return ""
	}

	serviceBlock := xmlContent[serviceStart : serviceStart+serviceEnd]

	// Extract controlURL
	controlStart := strings.Index(serviceBlock, "<controlURL>")
	if controlStart == -1 {
		return ""
	}
	controlStart += len("<controlURL>")

	controlEnd := strings.Index(serviceBlock[controlStart:], "</controlURL>")
	if controlEnd == -1 {
		return ""
	}

	return strings.TrimSpace(serviceBlock[controlStart : controlStart+controlEnd])
}

// xmlEscape escapes special characters for XML
func xmlEscape(s string) string {
	var buf bytes.Buffer
	xml.EscapeText(&buf, []byte(s))
	return buf.String()
}
