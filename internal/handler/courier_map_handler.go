package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/louissoe/niaga-autoparts/internal/model"
	"go.uber.org/zap"
)

// CourierMapHandler provides HTTP endpoints for viewing courier delivery maps.
type CourierMapHandler struct {
	logger *zap.Logger
}

// NewCourierMapHandler creates a new CourierMapHandler instance.
func NewCourierMapHandler(logger *zap.Logger) *CourierMapHandler {
	return &CourierMapHandler{logger: logger}
}

// RegisterPublicRoutes registers the courier map web view route (accessible to couriers).
func (h *CourierMapHandler) RegisterPublicRoutes(router *gin.Engine) {
	router.GET("/api/v1/courier/map-view", h.renderMapView)
	router.GET("/api/v1/courier/manifest-data", h.getManifestData)
}

func (h *CourierMapHandler) getManifestData(c *gin.Context) {
	task := model.GetDummyDeliveryTask()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    task,
	})
}

func (h *CourierMapHandler) renderMapView(c *gin.Context) {
	task := model.GetDummyDeliveryTask()
	taskJSON, err := json.Marshal(task)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error preparing map data")
		return
	}

	htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html lang="id">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <title>Peta Tugas Kurir - %s</title>
    <!-- Leaflet CSS -->
    <link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css" integrity="sha256-p4NxAoJBhIIN+hmNHrzRCf9tD/miZyoHS5obTRR9BMY=" crossorigin=""/>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700;800&display=swap" rel="stylesheet">
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
            font-family: 'Plus Jakarta Sans', -apple-system, BlinkMacSystemFont, sans-serif;
        }
        body, html {
            height: 100%%;
            width: 100%%;
            background-color: #0f172a;
            color: #f8fafc;
            overflow: hidden;
        }
        #app-container {
            display: flex;
            flex-direction: column;
            height: 100vh;
            width: 100vw;
        }
        header {
            background: linear-gradient(135deg, #1e293b, #0f172a);
            padding: 12px 16px;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
            display: flex;
            align-items: center;
            justify-content: space-between;
            z-index: 1000;
            box-shadow: 0 4px 20px rgba(0,0,0,0.3);
        }
        .header-title h1 {
            font-size: 16px;
            font-weight: 700;
            color: #38bdf8;
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .header-title p {
            font-size: 12px;
            color: #94a3b8;
        }
        .btn-nav-all {
            background: linear-gradient(135deg, #2563eb, #1d4ed8);
            color: white;
            padding: 8px 14px;
            border-radius: 8px;
            text-decoration: none;
            font-size: 12px;
            font-weight: 600;
            display: inline-flex;
            align-items: center;
            gap: 6px;
            box-shadow: 0 2px 10px rgba(37, 99, 235, 0.4);
            transition: all 0.2s ease;
        }
        .btn-nav-all:hover {
            transform: translateY(-1px);
            box-shadow: 0 4px 14px rgba(37, 99, 235, 0.6);
        }
        #map {
            flex: 1;
            width: 100%%;
            z-index: 1;
        }
        /* Custom Marker Badges */
        .custom-pin {
            display: flex;
            align-items: center;
            justify-content: center;
            border-radius: 50%%;
            color: white;
            font-weight: 800;
            font-size: 14px;
            box-shadow: 0 0 15px rgba(0,0,0,0.5), 0 0 0 3px white;
            cursor: pointer;
        }
        .origin-pin {
            background: #10b981;
            width: 36px;
            height: 36px;
            font-size: 18px;
        }
        .stop-pin {
            background: #ef4444;
            width: 32px;
            height: 32px;
        }
        /* Bottom sheet list */
        #bottom-panel {
            background: #1e293b;
            border-top: 1px solid rgba(255,255,255,0.1);
            max-height: 38vh;
            overflow-y: auto;
            padding: 12px;
            z-index: 1000;
        }
        .panel-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 8px;
            padding: 0 4px;
        }
        .panel-header h2 {
            font-size: 13px;
            font-weight: 700;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            color: #94a3b8;
        }
        .stop-card {
            background: #0f172a;
            border: 1px solid #334155;
            border-radius: 10px;
            padding: 10px 12px;
            margin-bottom: 8px;
            display: flex;
            align-items: flex-start;
            gap: 12px;
            cursor: pointer;
            transition: all 0.2s ease;
        }
        .stop-card:hover {
            border-color: #38bdf8;
            transform: translateX(2px);
        }
        .stop-badge {
            background: #ef4444;
            color: white;
            font-weight: 800;
            font-size: 12px;
            width: 26px;
            height: 26px;
            border-radius: 50%%;
            display: flex;
            align-items: center;
            justify-content: center;
            flex-shrink: 0;
            margin-top: 2px;
        }
        .stop-info {
            flex: 1;
        }
        .stop-info h3 {
            font-size: 13px;
            font-weight: 700;
            color: #f1f5f9;
        }
        .stop-info p {
            font-size: 11px;
            color: #94a3b8;
            margin-top: 2px;
            line-height: 1.3;
        }
        .stop-actions {
            display: flex;
            gap: 6px;
            margin-top: 8px;
        }
        .btn-action {
            font-size: 11px;
            padding: 4px 8px;
            border-radius: 6px;
            text-decoration: none;
            font-weight: 600;
        }
        .btn-maps {
            background: #0284c7;
            color: white;
        }
        .btn-wa {
            background: #16a34a;
            color: white;
        }
        /* Leaflet Popup Styling */
        .leaflet-popup-content-wrapper {
            background: #1e293b;
            color: #f8fafc;
            border-radius: 12px;
            border: 1px solid #475569;
            box-shadow: 0 10px 25px rgba(0,0,0,0.5);
        }
        .leaflet-popup-tip {
            background: #1e293b;
        }
        .popup-title {
            font-size: 13px;
            font-weight: 700;
            color: #38bdf8;
            margin-bottom: 4px;
        }
        .popup-address {
            font-size: 11px;
            color: #cbd5e1;
            margin-bottom: 6px;
        }
        .popup-items {
            font-size: 11px;
            color: #94a3b8;
            margin-bottom: 8px;
            font-style: italic;
        }
        .popup-btn {
            display: block;
            background: #2563eb;
            color: white !important;
            text-align: center;
            padding: 6px 10px;
            border-radius: 6px;
            text-decoration: none;
            font-size: 11px;
            font-weight: 600;
        }
    </style>
</head>
<body>
    <div id="app-container">
        <header>
            <div class="header-title">
                <h1>🚚 Rute Pengiriman Kurir</h1>
                <p>%s • Kurir: %s</p>
            </div>
            <a href="%s" target="_blank" class="btn-nav-all">
                🧭 Navigasi Google Maps
            </a>
        </header>

        <div id="map"></div>

        <div id="bottom-panel">
            <div class="panel-header">
                <h2>Daftar Titik Antar (%d Lokasi)</h2>
            </div>
            <div id="stops-list"></div>
        </div>
    </div>

    <!-- Leaflet JS -->
    <script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js" integrity="sha256-20nQCchB9co0qIjJZRGuk2/Z9VM+kNiyxNV1lvTlZBo=" crossorigin=""></script>
    <script>
        const taskData = %s;

        // Initialize Map
        const map = L.map('map').setView([taskData.origin_lat, taskData.origin_lng], 13);

        // Tile Layer (Dark Matter OSM)
        L.tileLayer('https://{s}.basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}{r}.png', {
            attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
            maxZoom: 19
        }).addTo(map);

        const markers = [];
        const latLngs = [];

        // 1. Origin (Gudang) Marker
        const originIcon = L.divIcon({
            className: 'custom-pin origin-pin',
            html: '🏢',
            iconSize: [36, 36],
            iconAnchor: [18, 18]
        });
        const originMarker = L.marker([taskData.origin_lat, taskData.origin_lng], { icon: originIcon })
            .bindPopup(
                '<div class="popup-title">🏢 ' + taskData.origin_name + '</div>' +
                '<div class="popup-address">Titik Awal Pengambilan Barang</div>'
            )
            .addTo(map);
        latLngs.push([taskData.origin_lat, taskData.origin_lng]);

        // 2. Stop Markers & List Population
        const stopsContainer = document.getElementById('stops-list');

        taskData.points.forEach((pt) => {
            const stopIcon = L.divIcon({
                className: 'custom-pin stop-pin',
                html: '<span>' + pt.sequence + '</span>',
                iconSize: [32, 32],
                iconAnchor: [16, 16]
            });

            const navUrl = 'https://www.google.com/maps/dir/?api=1&destination=' + pt.latitude + ',' + pt.longitude + '&travelmode=driving';
            const waPhone = pt.phone.startsWith('0') ? pt.phone.substring(1) : pt.phone;
            const waUrl = 'https://wa.me/62' + waPhone;

            const marker = L.marker([pt.latitude, pt.longitude], { icon: stopIcon })
                .bindPopup(
                    '<div class="popup-title">Stop #' + pt.sequence + ': ' + pt.customer_name + '</div>' +
                    '<div class="popup-address">' + pt.address + '</div>' +
                    '<div class="popup-items">📦 ' + pt.items_summary + '</div>' +
                    '<a href="' + navUrl + '" target="_blank" class="popup-btn">🧭 Buka Navigasi Rute</a>'
                )
                .addTo(map);

            markers.push(marker);
            latLngs.push([pt.latitude, pt.longitude]);

            // Add Stop Card
            const card = document.createElement('div');
            card.className = 'stop-card';
            card.innerHTML = 
                '<div class="stop-badge">' + pt.sequence + '</div>' +
                '<div class="stop-info">' +
                    '<h3>' + pt.customer_name + '</h3>' +
                    '<p>📦 ' + pt.order_number + '</p>' +
                    '<p>📍 ' + pt.address + '</p>' +
                    '<div class="stop-actions">' +
                        '<a href="' + navUrl + '" target="_blank" class="btn-action btn-maps">🧭 Rute</a>' +
                        '<a href="' + waUrl + '" target="_blank" class="btn-action btn-wa">💬 WA</a>' +
                    '</div>' +
                '</div>';
            card.addEventListener('click', (e) => {
                if (e.target.tagName !== 'A') {
                    map.flyTo([pt.latitude, pt.longitude], 16, { animate: true, duration: 1 });
                    marker.openPopup();
                }
            });
            stopsContainer.appendChild(card);
        });

        // 3. Draw Polyline Route connecting the stops
        const routeLine = L.polyline(latLngs, {
            color: '#38bdf8',
            weight: 4,
            opacity: 0.8,
            dashArray: '8, 8'
        }).addTo(map);

        // Fit map bounds to show all markers
        const group = L.featureGroup([originMarker, ...markers]);
        map.fitBounds(group.getBounds().pad(0.15));
    </script>
</body>
</html>`,
		task.ManifestID,
		task.ManifestID,
		task.CourierName,
		task.GenerateGoogleMapsRouteURL(),
		len(task.Points),
		string(taskJSON),
	)

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(htmlContent))
}
