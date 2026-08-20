package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// OSMService provides address geocoding via OpenStreetMap (Nominatim API).
type OSMService struct {
	client    *http.Client
	logger    *zap.Logger
	userAgent string
}

// NominatimSearchResult represents a single search result from OSM Nominatim.
type NominatimSearchResult struct {
	PlaceID     int64    `json:"place_id"`
	Licence     string   `json:"licence"`
	OsmType     string   `json:"osm_type"`
	OsmID       int64    `json:"osm_id"`
	BoundingBox []string `json:"boundingbox"`
	Lat         string   `json:"lat"`
	Lon         string   `json:"lon"`
	DisplayName string   `json:"display_name"`
	Class       string   `json:"class"`
	Type        string   `json:"type"`
	Importance  float64  `json:"importance"`
}

// NewOSMService creates a new OpenStreetMap Nominatim geocoding service.
func NewOSMService(logger *zap.Logger) *OSMService {
	return &OSMService{
		client: &http.Client{
			Timeout: 8 * time.Second,
		},
		logger:    logger,
		userAgent: "NiagaAutopartsBot/1.0 (contact: support@niaga-autoparts.local)",
	}
}

// Geocode converts an address string into latitude and longitude coordinates using OpenStreetMap Nominatim.
// Returns lat, lng, displayName, and error.
func (s *OSMService) Geocode(ctx context.Context, address string) (float64, float64, string, error) {
	if address == "" {
		return 0, 0, "", fmt.Errorf("empty address")
	}

	endpoint := fmt.Sprintf(
		"https://nominatim.openstreetmap.org/search?q=%s&format=json&addressdetails=1&limit=1&countrycodes=id",
		url.QueryEscape(address),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, 0, "", fmt.Errorf("create nominatim request: %w", err)
	}

	req.Header.Set("User-Agent", s.userAgent)
	req.Header.Set("Accept-Language", "id,en")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, 0, "", fmt.Errorf("nominatim http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, "", fmt.Errorf("nominatim returned status %d", resp.StatusCode)
	}

	var results []NominatimSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return 0, 0, "", fmt.Errorf("decode nominatim response: %w", err)
	}

	if len(results) == 0 {
		return 0, 0, "", fmt.Errorf("alamat tidak ditemukan di OpenStreetMap")
	}

	first := results[0]
	lat, errLat := strconv.ParseFloat(first.Lat, 64)
	lng, errLng := strconv.ParseFloat(first.Lon, 64)
	if errLat != nil || errLng != nil {
		return 0, 0, "", fmt.Errorf("invalid coordinates from nominatim: lat=%s, lon=%s", first.Lat, first.Lon)
	}

	s.logger.Info("OSM geocode success",
		zap.String("query", address),
		zap.Float64("lat", lat),
		zap.Float64("lng", lng),
		zap.String("display_name", first.DisplayName),
	)

	return lat, lng, first.DisplayName, nil
}
