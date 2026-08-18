package utils

import "math"

const (
	// EarthRadiusKm is the approximate radius of Earth in kilometers.
	EarthRadiusKm = 6371.0
)

// Coordinates represents a geographical latitude and longitude pair.
type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// degreesToRadians converts angle in degrees to radians.
func degreesToRadians(degrees float64) float64 {
	return degrees * math.Pi / 180.0
}

// CalculateHaversineDistanceKm computes great-circle distance between two coordinates in kilometers.
func CalculateHaversineDistanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := degreesToRadians(lat2 - lat1)
	dLon := degreesToRadians(lon2 - lon1)

	rLat1 := degreesToRadians(lat1)
	rLat2 := degreesToRadians(lat2)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(rLat1)*math.Cos(rLat2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return EarthRadiusKm * c
}
