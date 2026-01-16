package dlna

import (
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/wysentanu/dlna-movie-cast/internal/library"
)

// ContentDirectoryService handles ContentDirectory SOAP actions
type ContentDirectoryService struct {
	library    *library.Library
	serverAddr string
	updateID   uint32
}

// NewContentDirectoryService creates a new ContentDirectory service
func NewContentDirectoryService(lib *library.Library, serverAddr string) *ContentDirectoryService {
	return &ContentDirectoryService{
		library:    lib,
		serverAddr: serverAddr,
		updateID:   1,
	}
}

// SOAPEnvelope represents a SOAP envelope
type SOAPEnvelope struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	Body    SOAPBody `xml:"Body"`
}

// SOAPBody represents the SOAP body
type SOAPBody struct {
	Content []byte `xml:",innerxml"`
}

// BrowseRequest represents a Browse SOAP action request
type BrowseRequest struct {
	ObjectID       string `xml:"ObjectID"`
	BrowseFlag     string `xml:"BrowseFlag"`
	Filter         string `xml:"Filter"`
	StartingIndex  int    `xml:"StartingIndex"`
	RequestedCount int    `xml:"RequestedCount"`
	SortCriteria   string `xml:"SortCriteria"`
}

// HandleControl handles SOAP control requests
func (s *ContentDirectoryService) HandleControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse SOAP action from header
	soapAction := r.Header.Get("SOAPAction")
	soapAction = strings.Trim(soapAction, "\"")

	var response string

	switch {
	case strings.Contains(soapAction, "Browse"):
		response = s.handleBrowse(body)
	case strings.Contains(soapAction, "GetSystemUpdateID"):
		response = s.handleGetSystemUpdateID()
	case strings.Contains(soapAction, "GetSearchCapabilities"):
		response = s.handleGetSearchCapabilities()
	case strings.Contains(soapAction, "GetSortCapabilities"):
		response = s.handleGetSortCapabilities()
	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.Write([]byte(response))
}

// handleBrowse handles the Browse action
func (s *ContentDirectoryService) handleBrowse(body []byte) string {
	// Parse the Browse request
	var req BrowseRequest

	// Extract the Browse element from SOAP body
	bodyStr := string(body)

	// Simple XML parsing for the Browse parameters
	req.ObjectID = extractXMLValue(bodyStr, "ObjectID")
	req.BrowseFlag = extractXMLValue(bodyStr, "BrowseFlag")
	req.Filter = extractXMLValue(bodyStr, "Filter")
	req.StartingIndex, _ = strconv.Atoi(extractXMLValue(bodyStr, "StartingIndex"))
	req.RequestedCount, _ = strconv.Atoi(extractXMLValue(bodyStr, "RequestedCount"))
	req.SortCriteria = extractXMLValue(bodyStr, "SortCriteria")

	// Default values
	if req.RequestedCount == 0 {
		req.RequestedCount = 100
	}

	var didl string
	var numberReturned, totalMatches int

	switch req.ObjectID {
	case "0":
		// Root container
		if req.BrowseFlag == "BrowseMetadata" {
			didl = s.buildRootContainerMetadata()
			numberReturned = 1
			totalMatches = 1
		} else {
			didl, numberReturned, totalMatches = s.buildRootContainerChildren(req.StartingIndex, req.RequestedCount)
		}
	case "movies":
		// Movies container
		if req.BrowseFlag == "BrowseMetadata" {
			didl = s.buildMoviesContainerMetadata()
			numberReturned = 1
			totalMatches = 1
		} else {
			didl, numberReturned, totalMatches = s.buildMoviesList(req.StartingIndex, req.RequestedCount)
		}
	default:
		// Specific item
		didl, numberReturned, totalMatches = s.buildItemMetadata(req.ObjectID)
	}

	return s.wrapBrowseResponse(didl, numberReturned, totalMatches)
}

// buildRootContainerMetadata builds the root container metadata
func (s *ContentDirectoryService) buildRootContainerMetadata() string {
	return `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">
<container id="0" parentID="-1" restricted="1" searchable="0">
<dc:title>Root</dc:title>
<upnp:class>object.container</upnp:class>
</container>
</DIDL-Lite>`
}

// buildRootContainerChildren builds the root container children
func (s *ContentDirectoryService) buildRootContainerChildren(start, count int) (string, int, int) {
	didl := `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">
<container id="movies" parentID="0" restricted="1" childCount="` + strconv.Itoa(len(s.library.GetAllMovies())) + `">
<dc:title>Movies</dc:title>
<upnp:class>object.container.storageFolder</upnp:class>
</container>
</DIDL-Lite>`

	return didl, 1, 1
}

// buildMoviesContainerMetadata builds the movies container metadata
func (s *ContentDirectoryService) buildMoviesContainerMetadata() string {
	movies := s.library.GetAllMovies()
	return fmt.Sprintf(`<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">
<container id="movies" parentID="0" restricted="1" childCount="%d">
<dc:title>Movies</dc:title>
<upnp:class>object.container.storageFolder</upnp:class>
</container>
</DIDL-Lite>`, len(movies))
}

// buildMoviesList builds the list of movies
func (s *ContentDirectoryService) buildMoviesList(start, count int) (string, int, int) {
	movies := s.library.GetAllMovies()
	totalMatches := len(movies)

	// Apply pagination
	end := start + count
	if end > len(movies) {
		end = len(movies)
	}
	if start > len(movies) {
		start = len(movies)
	}

	paginatedMovies := movies[start:end]

	var items strings.Builder
	items.WriteString(`<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/">`)

	for _, movie := range paginatedMovies {
		items.WriteString(s.buildMovieItem(movie))
	}

	items.WriteString(`</DIDL-Lite>`)

	return items.String(), len(paginatedMovies), totalMatches
}

// buildMovieItem builds a single movie item
func (s *ContentDirectoryService) buildMovieItem(movie *library.Movie) string {
	title := html.EscapeString(movie.Title)
	streamURL := fmt.Sprintf("%s/stream/%s", s.serverAddr, movie.ID)

	// Format duration as HH:MM:SS
	hours := movie.Duration / 3600
	minutes := (movie.Duration % 3600) / 60
	seconds := movie.Duration % 60
	duration := fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)

	// Determine DLNA profile based on codec
	dlnaProfile := "DLNA.ORG_PN=AVC_MP4_MP_SD_AAC_MULT5"
	if movie.VideoCodec == "hevc" || movie.VideoCodec == "h265" {
		dlnaProfile = "DLNA.ORG_PN=HEVC_Main10_L5"
	}

	resolution := fmt.Sprintf("%dx%d", movie.VideoWidth, movie.VideoHeight)

	return fmt.Sprintf(`
<item id="%s" parentID="movies" restricted="1">
<dc:title>%s</dc:title>
<upnp:class>object.item.videoItem.movie</upnp:class>
<res protocolInfo="http-get:*:video/mp4:%s;DLNA.ORG_OP=01;DLNA.ORG_FLAGS=01700000000000000000000000000000" size="%d" duration="%s" resolution="%s">%s</res>
</item>`,
		movie.ID,
		title,
		dlnaProfile,
		movie.FileSize,
		duration,
		resolution,
		html.EscapeString(streamURL),
	)
}

// buildItemMetadata builds metadata for a specific item
func (s *ContentDirectoryService) buildItemMetadata(objectID string) (string, int, int) {
	movie, err := s.library.GetMovie(objectID)
	if err != nil {
		return `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"></DIDL-Lite>`, 0, 0
	}

	didl := `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/">`
	didl += s.buildMovieItem(movie)
	didl += `</DIDL-Lite>`

	return didl, 1, 1
}

// wrapBrowseResponse wraps the DIDL-Lite in a SOAP Browse response
func (s *ContentDirectoryService) wrapBrowseResponse(didl string, numberReturned, totalMatches int) string {
	escapedDIDL := html.EscapeString(didl)

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:BrowseResponse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">
<Result>%s</Result>
<NumberReturned>%d</NumberReturned>
<TotalMatches>%d</TotalMatches>
<UpdateID>%d</UpdateID>
</u:BrowseResponse>
</s:Body>
</s:Envelope>`, escapedDIDL, numberReturned, totalMatches, s.updateID)
}

// handleGetSystemUpdateID handles the GetSystemUpdateID action
func (s *ContentDirectoryService) handleGetSystemUpdateID() string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:GetSystemUpdateIDResponse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">
<Id>%d</Id>
</u:GetSystemUpdateIDResponse>
</s:Body>
</s:Envelope>`, s.updateID)
}

// handleGetSearchCapabilities handles the GetSearchCapabilities action
func (s *ContentDirectoryService) handleGetSearchCapabilities() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:GetSearchCapabilitiesResponse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">
<SearchCaps></SearchCaps>
</u:GetSearchCapabilitiesResponse>
</s:Body>
</s:Envelope>`
}

// handleGetSortCapabilities handles the GetSortCapabilities action
func (s *ContentDirectoryService) handleGetSortCapabilities() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:GetSortCapabilitiesResponse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">
<SortCaps>dc:title</SortCaps>
</u:GetSortCapabilitiesResponse>
</s:Body>
</s:Envelope>`
}

// IncrementUpdateID increments the system update ID (call after library changes)
func (s *ContentDirectoryService) IncrementUpdateID() {
	s.updateID++
}

// Helper function to extract XML values
func extractXMLValue(xml, tag string) string {
	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"

	start := strings.Index(xml, startTag)
	if start == -1 {
		return ""
	}
	start += len(startTag)

	end := strings.Index(xml[start:], endTag)
	if end == -1 {
		return ""
	}

	return xml[start : start+end]
}
