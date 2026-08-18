package service

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "go.uber.org/zap"
)

// GoogleMapsService provides distance calculation via Google Maps Distance Matrix API.
type GoogleMapsService struct {
    apiKey string
    client *http.Client
    logger *zap.Logger
}

// NewGoogleMapsService creates a new service instance.
func NewGoogleMapsService(apiKey string) *GoogleMapsService {
    return &GoogleMapsService{
        apiKey: apiKey,
        client: &http.Client{Timeout: 5 * time.Second},
        logger: zap.L(),
    }
}

// distanceMatrixResponse models the JSON response from the Distance Matrix API.
type distanceMatrixResponse struct {
    Status string `json:"status"`
    Rows   []struct {
        Elements []struct {
            Status   string `json:"status"`
            Distance struct {
                Value int `json:"value"` // meters
            } `json:"distance"`
        } `json:"elements"`
    } `json:"rows"`
}

// GetDrivingDistance returns the road distance in kilometers between origin and destination.
func (s *GoogleMapsService) GetDrivingDistance(ctx context.Context, originLat, originLng, destLat, destLng float64) (float64, error) {
    url := fmt.Sprintf(
        "https://maps.googleapis.com/maps/api/distancematrix/json?origins=%f,%f&destinations=%f,%f&key=%s",
        originLat, originLng, destLat, destLng, s.apiKey,
    )
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return 0, err
    }
    resp, err := s.client.Do(req)
    if err != nil {
        return 0, err
    }
    defer resp.Body.Close()
    var rawResp map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&rawResp); err != nil {
        return 0, err
    }

    status, _ := rawResp["status"].(string)
    if status != "OK" {
        errMsg, _ := rawResp["error_message"].(string)
        return 0, fmt.Errorf("google maps api error (status: %s, message: %s)", status, errMsg)
    }

    rows, ok := rawResp["rows"].([]interface{})
    if !ok || len(rows) == 0 {
        return 0, fmt.Errorf("no rows found in google maps response")
    }

    firstRow, ok := rows[0].(map[string]interface{})
    if !ok {
        return 0, fmt.Errorf("invalid row format")
    }

    elements, ok := firstRow["elements"].([]interface{})
    if !ok || len(elements) == 0 {
        return 0, fmt.Errorf("no elements found in google maps response")
    }

    element, ok := elements[0].(map[string]interface{})
    if !ok {
        return 0, fmt.Errorf("invalid element format")
    }

    elemStatus, _ := element["status"].(string)
    if elemStatus != "OK" {
        return 0, fmt.Errorf("distance element status not OK: %s", elemStatus)
    }

    distanceMap, ok := element["distance"].(map[string]interface{})
    if !ok {
        return 0, fmt.Errorf("distance field missing")
    }

    valueFloat, ok := distanceMap["value"].(float64)
    if !ok {
        return 0, fmt.Errorf("distance value invalid")
    }

    km := valueFloat / 1000.0
    return km, nil
}
