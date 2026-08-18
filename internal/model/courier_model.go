package model

import (
	"fmt"

	"github.com/louissoe/niaga-autoparts/internal/utils"
)

// DeliveryPoint represents a single stop/destination for the courier.
type DeliveryPoint struct {
	Sequence     int     `json:"sequence"`
	OrderNumber  string  `json:"order_number"`
	CustomerName string  `json:"customer_name"`
	Phone        string  `json:"phone"`
	Address      string  `json:"address"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	Notes        string  `json:"notes"`
	ItemsSummary string  `json:"items_summary"`
}

// DeliveryTask represents a courier's batch/manifest of deliveries.
type DeliveryTask struct {
	ManifestID  string          `json:"manifest_id"`
	CourierName string          `json:"courier_name"`
	Date        string          `json:"date"`
	OriginName  string          `json:"origin_name"`
	OriginLat   float64         `json:"origin_lat"`
	OriginLng   float64         `json:"origin_lng"`
	Points      []DeliveryPoint `json:"points"`
}

// GetLocation implements utils.DeliveryRoutable.
func (p DeliveryPoint) GetLocation() (float64, float64) {
	return p.Latitude, p.Longitude
}

// GetDummyDeliveryTask returns sample delivery points for demonstration (automatically route-optimized).
func GetDummyDeliveryTask() DeliveryTask {
	// Sample delivery points (purposely listed in unordered sequence to test optimization)
	rawTask := DeliveryTask{
		ManifestID:  "MNF-20260815-001",
		CourierName: "Budi Santoso",
		Date:        "15 Agustus 2026",
		OriginName:  "Gudang Pusat Niaga Autoparts (Sunter)",
		OriginLat:   -6.1384,
		OriginLng:   106.8640,
		Points: []DeliveryPoint{
			{
				OrderNumber:  "ORD-20260815-003",
				CustomerName: "Garasi Modif 99 (Mas Bayu)",
				Phone:        "081311223344",
				Address:      "Jl. Pemuda No. 45, Rawamangun, Jakarta Timur",
				Latitude:     -6.1925,
				Longitude:    106.8856,
				Notes:        "Telepon dulu jika sudah sampai di gerbang.",
				ItemsSummary: "1x Shockbreaker Belakang Kayaba (Sepasang)",
			},
			{
				OrderNumber:  "ORD-20260815-001",
				CustomerName: "Bengkel Jaya Motor (Pak Hendra)",
				Phone:        "081234567890",
				Address:      "Jl. Danau Sunter Utara No. 12, Jakarta Utara",
				Latitude:     -6.1352,
				Longitude:    106.8589,
				Notes:        "Antar sebelum jam 12 siang. Titip di kasir.",
				ItemsSummary: "2x Kampas Rem Depan Avanza, 1x Oli Shell Helix 4L",
			},
			{
				OrderNumber:  "ORD-20260815-004",
				CustomerName: "Auto Care Service (Pak Rian)",
				Phone:        "081566778899",
				Address:      "Jl. Matraman Raya No. 88, Jakarta Timur",
				Latitude:     -6.2058,
				Longitude:    106.8581,
				Notes:        "Gedung 2 lantai, masuk ke workshop bagian belakang.",
				ItemsSummary: "3x Radiator Coolant Prestone, 2x Filter Oli",
			},
			{
				OrderNumber:  "ORD-20260815-002",
				CustomerName: "Toko Sparepart Sentosa (Bu Linda)",
				Phone:        "081298765432",
				Address:      "Jl. Boulevard Barat Raya Blok LC6 No. 25, Kelapa Gading",
				Latitude:     -6.1518,
				Longitude:    106.8924,
				Notes:        "Pagar abu-abu sebelah bank BCA.",
				ItemsSummary: "4x Busi Iridium Denso, 2x Filter Udara Innova",
			},
		},
	}

	rawTask.OptimizeRoute()
	return rawTask
}

// OptimizeRoute re-sequences Points to minimize total traveling distance from Origin.
func (d *DeliveryTask) OptimizeRoute() {
	if len(d.Points) <= 1 {
		for i := range d.Points {
			d.Points[i].Sequence = i + 1
		}
		return
	}

	routablePoints := make([]utils.DeliveryRoutable, len(d.Points))
	for i, pt := range d.Points {
		routablePoints[i] = pt
	}

	// Calculate optimal order indices using Nearest Neighbor + 2-Opt TSP
	optimizedIndices := utils.OptimizeRouteNearestNeighbor(d.OriginLat, d.OriginLng, routablePoints)

	newPoints := make([]DeliveryPoint, len(d.Points))
	for newSeq, originalIdx := range optimizedIndices {
		point := d.Points[originalIdx]
		point.Sequence = newSeq + 1
		newPoints[newSeq] = point
	}

	d.Points = newPoints
}

// GenerateGoogleMapsRouteURL creates a multi-stop Google Maps directions URL.
func (d *DeliveryTask) GenerateGoogleMapsRouteURL() string {
	if len(d.Points) == 0 {
		return ""
	}

	origin := fmt.Sprintf("%f,%f", d.OriginLat, d.OriginLng)
	lastPoint := d.Points[len(d.Points)-1]
	destination := fmt.Sprintf("%f,%f", lastPoint.Latitude, lastPoint.Longitude)

	var waypoints string
	if len(d.Points) > 1 {
		for i := 0; i < len(d.Points)-1; i++ {
			if i > 0 {
				waypoints += "|"
			}
			waypoints += fmt.Sprintf("%f,%f", d.Points[i].Latitude, d.Points[i].Longitude)
		}
	}

	if waypoints != "" {
		return fmt.Sprintf("https://www.google.com/maps/dir/?api=1&origin=%s&destination=%s&waypoints=%s&travelmode=driving", origin, destination, waypoints)
	}

	return fmt.Sprintf("https://www.google.com/maps/dir/?api=1&origin=%s&destination=%s&travelmode=driving", origin, destination)
}
