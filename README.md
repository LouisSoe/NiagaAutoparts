# WhatsApp AutoParts Chatbot 🔧🚗

Chatbot WhatsApp siap produksi untuk toko suku cadang otomotif.  
Dibangun dengan **Go**, **Fonnte** (WhatsApp Gateway), **Telegram Bot**, dan **Google Gemini Flash Lite** (AI).  
Database menggunakan **PostgreSQL** dengan ekstensi `pg_trgm` untuk pencarian pintar.
Cache menggunakan **in-memory native Go** — tanpa Redis, tanpa dependensi eksternal tambahan.
Dilengkapi juga dengan **Marketplace Scraper** menggunakan Go-Rod untuk perbandingan harga otomatis.

---

## Daftar Isi

- [Prasyarat](#prasyarat)
- [Cara Mendapatkan API Key](#cara-mendapatkan-api-key)
- [Instalasi & Setup](#instalasi--setup)
- [Menjalankan Aplikasi](#menjalankan-aplikasi)
- [Konfigurasi Webhook Fonnte](#konfigurasi-webhook-fonnte)
- [Cek Koneksi Fonnte](#cek-koneksi-fonnte)
- [Cara Pakai (Perintah Chat)](#cara-pakai-perintah-chat)
- [Arsitektur Sistem](#arsitektur-sistem)
- [Struktur Folder](#struktur-folder)
- [API Endpoints](#api-endpoints)
- [Environment Variables](#environment-variables)
- [Performa](#performa)
- [Checklist Produksi](#checklist-produksi)

---

## Prasyarat

Pastikan sudah terinstall di komputer/server kamu:

| Tools | Versi Minimum | Cek |
|-------|---------------|-----|
| Go | 1.24+ | `go version` |
| PostgreSQL | 12.0+ | `psql --version` |
| Git | any | `git --version` |
| ngrok *(lokal)* | any | `ngrok version` |

> **Redis tidak diperlukan.** Cache menggunakan `sync.RWMutex` + background eviction goroutine bawaan Go.

---

## Cara Mendapatkan API Key

### 1. Fonnte (WhatsApp Gateway)

1. Daftar di [fonnte.com](https://fonnte.com)
2. Login → klik **Devices** → **Add Device**
3. Scan QR Code dengan WhatsApp yang akan dipakai sebagai bot
4. Setelah terhubung, klik **Token** — copy token tersebut
5. Masukkan ke `.env` sebagai `FONNTE_TOKEN`

> ⚠️ Nomor WhatsApp yang di-scan **tidak bisa dipakai chat biasa** selama aktif sebagai bot. Gunakan nomor khusus.

### 2. Google AI Studio (Gemini)

1. Buka [aistudio.google.com](https://aistudio.google.com)
2. Login dengan Google Account
3. Klik **Get API Key** → **Create API Key**
4. Copy key-nya, masukkan ke `.env` sebagai `GEMINI_API_KEY`

> Gemini Flash Lite tersedia di free tier — tidak perlu kartu kredit untuk development.

### 3. Telegram Bot Ecosystem (3 Bot)

Aplikasi ini menggunakan 3 Bot Telegram terpisah untuk kebutuhan operasional:
1. **Bot 1 (Customer Service Chatbot):** Bot interaktif pelanggan untuk pencarian barang, katalog, reservasi pesanan, dan pengantaran kurir (`TELE_API`).
2. **Bot 2 (System Notifier & Broadcaster):** Bot untuk broadcast pesanan baru ke channel admin (`TELEGRAM_ORDER_CHANNEL_ID`) dan error alert (`TELEGRAM_ERROR_CHANNEL_ID`).
3. **Bot 3 (Courier Delivery Assistant):** Asisten kurir untuk menerima notifikasi order delivery, navigasi rute Google Maps / Web Leaflet, dan reminder jadwal pengantaran (`TELEGRAM_COURIER_BOT_TOKEN`).

#### Cara Menghubungkan Akun Kurir ke Bot 3:
1. Pastikan akun kurir sudah terdaftar dengan role `courier` (misal via POST `/api/v1/users`).
2. Buka Bot Kurir Anda di Telegram (`@CourierAutopart_bot`).
3. Ketik perintah:
   ```text
   /link <email_kurir>
   ```
   *(Contoh: `/link kurir1@niagagudang.com`)*
4. Akun kurir akan otomatis terhubung dengan ID Telegram dan mulai menerima notifikasi order delivery serta reminder jadwal pengiriman.

---

## Instalasi & Setup

### 1. Clone Repository

```bash
git clone https://github.com/louissoe/niaga-autoparts.git
cd niaga-autoparts
```

### 2. Install Dependensi Go

```bash
go mod download
```

### 3. Buat File `.env`

```bash
cp .env.example .env
```

Edit `.env` dengan text editor favoritmu:

```bash
nano .env
# atau
code .env
```

Isi minimal yang wajib diisi:

```env
# Database (PostgreSQL)
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASS=password_postgres_kamu
DB_NAME=autoparts_db

# Fonnte
FONNTE_TOKEN=token_dari_fonnte_kamu

# Gemini
GEMINI_API_KEY=key_dari_google_ai_studio_kamu

# Telegram (Opsional)
TELE_API=token_dari_botfather_kamu
```

### 4. Setup Database

Buat database dan jalankan migrasi:

```bash
# Buat database dulu (kalau belum ada)
psql -U postgres -c "CREATE DATABASE autoparts_db;"

# Jalankan schema + seed data
psql -U postgres -d autoparts_db -f migrations/001_init.sql
```

Verifikasi data seed masuk:

```bash
psql -U postgres -d autoparts_db -c "SELECT id, sku, name, price, stock FROM products LIMIT 5;"
```

Output yang diharapkan:
```
+----+-------------+-------------------------------+-----------+-------+
| id | sku         | name                          | price     | stock |
+----+-------------+-------------------------------+-----------+-------+
|  1 | KR-HB-001   | Kampas Rem Honda Beat Depan   | 35000.00  |    15 |
|  2 | KR-HB-002   | Kampas Rem Honda Beat Belakang| 28000.00  |    12 |
|  3 | FO-AHM-001  | Filter Oli Honda Beat/Vario   | 22000.00  |    30 |
...
```

---

## Menjalankan Aplikasi

### Mode Development

```bash
go run ./cmd/server/
```

Output yang muncul:
```
2024-01-15T10:00:00.000Z  INFO  configuration loaded  {"env": "development"}
2024-01-15T10:00:00.001Z  INFO  database connected
2024-01-15T10:00:00.002Z  INFO  in-memory cache started  {"product_ttl": "1m0s", "cleanup_interval": "2m0s"}
2024-01-15T10:00:00.003Z  INFO  starting worker pool  {"workers": 10}
2024-01-15T10:00:00.004Z  INFO  server starting  {"port": "8080"}
```

### Mode Production (Build Binary)

```bash
# Build
go build -o bin/autoparts ./cmd/server/

# Jalankan
APP_ENV=production ./bin/autoparts
```

### Dengan Air (Hot Reload, opsional)

```bash
# Install air
go install github.com/air-verse/air@latest

# Jalankan dengan auto-reload saat file berubah
air
```

### Test Health Check

```bash
curl http://localhost:8080/health
# Output: {"status":"ok","time":"2024-01-15T10:00:00Z"}
```

---

## Konfigurasi Webhook Fonnte

Webhook adalah URL yang dipanggil Fonnte setiap kali ada pesan masuk ke nomor bot kamu.

### Development (Local) — Pakai ngrok

Karena server local tidak punya IP publik, kita pakai ngrok sebagai tunnel:

```bash
# Terminal 1 — jalankan server
go run ./cmd/server/

# Terminal 2 — buka tunnel ngrok
ngrok http 8080
```

ngrok akan tampil seperti ini:
```
Forwarding  https://abc123xyz.ngrok-free.app -> http://localhost:8080
```

Copy URL `https://abc123xyz.ngrok-free.app/webhook` lalu:

1. Login ke **dashboard.fonnte.com**
2. Klik device kamu → **Settings**
3. Isi field **Webhook URL**: `https://abc123xyz.ngrok-free.app/webhook`
4. Klik **Save**

> ⚠️ URL ngrok berubah setiap kali ngrok di-restart (kecuali pakai plan berbayar). Update webhook di Fonnte setiap restart ngrok.

### Production — Server dengan IP Publik

Kalau sudah deploy ke VPS/cloud:

```bash
# Contoh URL webhook production
https://yourdomain.com/webhook

# Atau pakai IP langsung
http://123.456.789.000:8080/webhook
```

Set di Fonnte dashboard sama seperti di atas.

---

## Cek Koneksi Fonnte

Kamu bisa mengetes apakah token Fonnte kamu valid dan server bisa mengirim pesan dengan endpoint `/fonnte/test`.

### 1. Test via Browser/Curl

```bash
# Kirim pesan test ke nomormu sendiri
curl "http://localhost:8080/fonnte/test?target=6281234567890"

# Atau dengan pesan custom
curl "http://localhost:8080/fonnte/test?target=6281234567890&message=Halo+dari+NiagaGudang"
```

### 2. Response yang Diharapkan

Jika sukses, kamu akan menerima pesan WhatsApp dan response JSON seperti ini:
```json
{
  "ok": true,
  "detail": "success",
  "id": "xxxx",
  "target": "6281234567890",
  "process": "..."
}
```

Jika gagal (misal token salah):
```json
{
  "ok": false,
  "error": "fonnte api error: 401 ..."
}
```

---

### Test Webhook Manual

```bash
curl -X POST http://localhost:8080/webhook \
  -d "sender=6281234567890" \
  -d "message=halo" \
  -d "name=Budi" \
  -d "device=6289876543210"
```

Response yang diharapkan:
```json
{"status": "queued"}
```

---

## Cara Pakai (Perintah Chat)

Kirim pesan berikut ke nomor WhatsApp bot:

### 👋 Salam & Menu Bantuan

| Perintah | Deskripsi |
|----------|-----------|
| `halo` / `hi` / `assalamualaikum` | Tampilkan pesan sambutan |
| `menu` / `help` / `bantuan` | Tampilkan semua perintah |

**Contoh balasan:**
```
Halo Budi! 👋

Selamat datang di Toko Sparepart Auto 🔧🚗

Silakan ketik:
• CARI [nama produk] — Cari produk
• HARGA [nama produk] — Cek harga
• ORDER [nama produk] [jumlah] — Pesan
• MENU — Bantuan

Atau kirim foto suku cadang untuk identifikasi otomatis! 📷
```

---

### 🔍 Cari Produk

| Perintah | Contoh |
|----------|--------|
| `cari [nama]` | `cari kampas rem` |
| `ada [nama]` | `ada filter oli honda` |
| `stok [nama]` | `stok busi ngk` |

**Hasil pencarian banyak produk:**
```
✅ Ditemukan 2 produk:

1. Kampas Rem Honda Beat Depan
   SKU: KR-HB-001
   Harga: Rp 35.000 / pcs
   Lokasi: Rak A1-B1
   Stok: Ada Stok ✅

2. Kampas Rem Honda Beat Belakang
   SKU: KR-HB-002
   Harga: Rp 28.000 / pcs
   Lokasi: Rak A1-B2
   Stok: Ada Stok ✅

Balas dengan nomor produk untuk detail, atau PESAN [no] [jumlah] untuk memesan.
```

**Hasil pencarian satu produk (langsung detail + harga marketplace):**
```
🔧 Filter Oli Honda Beat/Vario
━━━━━━━━━━━━━━━━
SKU      : FO-AHM-001
Kategori : Filter
Harga    : Rp 22.000 / pcs
Stok     : ✅ Tersedia (30 pcs tersedia)
Lokasi   : Rak B2-C3

📊 Perbandingan Harga Marketplace:
  • Tokopedia: Rp 23.100 💰 Lebih murah di sini
  • Shopee: Rp 22.660 💰 Lebih murah di sini

Ketik PESAN [jumlah] untuk memesan produk ini.
```

---

### 💰 Cek Harga

| Perintah | Contoh |
|----------|--------|
| `harga [nama]` | `harga busi ngk` |
| `berapa [nama]` | `berapa harga aki` |

Sama seperti pencarian produk, tapi lebih fokus ke info harga dan perbandingan marketplace.

---

### 🛒 Pesan Produk (2-Step Flow)

**Step 1 — Buat reservasi:**

| Perintah | Contoh |
|----------|--------|
| `pesan [nama] [jumlah]` | `pesan kampas rem honda beat depan 2` |
| `order [nama] [jumlah]` | `order filter oli 1` |
| `beli [nama] [jumlah]` | `beli busi ngk 4` |

Bot akan meminta konfirmasi:
```
🛒 Konfirmasi Pesanan
━━━━━━━━━━━━━━━━
No. Pesanan : APT-20240115-A3F9
Produk      : Kampas Rem Honda Beat Depan
Jumlah      : 2 pcs
Harga Satuan: Rp 35.000
Total       : Rp 70.000
━━━━━━━━━━━━━━━━
⏰ Reservasi berlaku 15 menit.

Balas YA untuk konfirmasi atau BATAL untuk membatalkan.
```

> Saat konfirmasi dikirim, stok sudah **direservasi** — pengguna lain tidak bisa memesan produk yang sama melampaui stok tersisa.

**Step 2 — Konfirmasi atau batalkan:**

| Perintah | Deskripsi |
|----------|-----------|
| `ya` / `iya` / `ok` / `setuju` | Konfirmasi pesanan → stok dipotong permanen |
| `batal` / `tidak` / `cancel` | Batalkan pesanan → stok dilepas kembali |

**Setelah konfirmasi:**
```
✅ Pesanan Dikonfirmasi!

No. Pesanan : APT-20240115-A3F9
Produk      : Kampas Rem Honda Beat Depan
Total       : Rp 70.000

Tim kami akan segera memproses pesanan Anda.
Lokasi pengambilan akan diinformasikan oleh admin.

Terima kasih! 🙏
```

> Jika tidak ada konfirmasi dalam **15 menit**, reservasi otomatis dibatalkan dan stok dikembalikan.

---

### 📦 Cek Status Pesanan

| Perintah | Contoh |
|----------|--------|
| `cek order` / `status order` | Lihat semua pesanan terakhir |
| `order saya` / `pesanan saya` | Sama seperti di atas |

```
📦 Pesanan Anda:

✅ APT-20240115-A3F9 — Kampas Rem Honda Beat Depan (x2) — Rp 70.000
🔒 APT-20240116-B7K2 — Filter Oli Honda Beat (x1) — Rp 22.000
```

**Keterangan status:**
| Ikon | Status | Arti |
|------|--------|------|
| ⏳ | pending | Menunggu konfirmasi |
| 🔒 | reserved | Stok direservasi, menunggu konfirmasi final |
| ✅ | paid | Pesanan dikonfirmasi, sedang diproses |
| ❌ | cancelled | Dibatalkan |

---

### 📷 Identifikasi Foto Suku Cadang

Kirim **foto** suku cadang langsung ke chat (tanpa teks) — AI akan mencoba mengidentifikasi produknya.

```
[User mengirim foto kampas rem]

Bot: ⏳ Sedang diproses...

Bot: 🔍 Hasil Identifikasi Foto:

     Kemungkinan produk:
       1. Kampas Rem Cakram
       2. Brake Pad Semi-Metallic
     
     Ketik CARI [nama produk] untuk mencari di database kami.
```

Lanjutkan dengan:
```
cari kampas rem cakram
```

---

## Arsitektur Sistem

```
Fonnte (WhatsApp) / Telegram Bot
       │  POST /webhook
       ▼
  ┌──────────────┐   goroutine    ┌──────────────────┐
  │   Webhook    │ ─────────────► │   Worker Pool    │
  │   Handler    │  ACK < 1 detik │  (N goroutines)  │
  └──────────────┘                └────────┬─────────┘
                                            │
                              ┌─────────────▼────────────────┐
                              │       MessageProcessor        │
                              │                               │
                              │  1. Load/Create Session       │
                              │  2. Detect Intent             │
                              │     ├─ Rule-based (0ms)       │
                              │     └─ Gemini AI (fallback)   │
                              │  3. Business Logic            │
                              │     ├─ Product Search + Cache │
                              │     ├─ Order Reserve/Confirm  │
                              │     └─ Image Identify (AI)    │
                              │  4. Send Reply via Fonnte     │
                              └───────────────────────────────┘

Cache Layer (in-memory, no Redis):
  ┌─────────────────────────────────────┐
  │  sync.RWMutex + map[string]entry    │
  │  TTL per-entry, background eviction │
  └─────────────────────────────────────┘
```

### Two-Step Response Strategy

| Step | Target | Aksi |
|------|--------|------|
| 1 | < 1 detik | Kirim `⏳ Sedang diproses...` ke user |
| 2 | < 5 detik | Kirim balasan final setelah proses selesai |

Fonnte menerima HTTP 200 langsung → tidak ada retry dari gateway.

---

## Struktur Folder

```
niaga-autoparts/
├── cmd/
│   └── main.go                       # Entry point, wiring semua komponen
├── internal/
│   ├── config/
│   │   └── config.go                 # Load env vars ke struct Config
│   ├── model/
│   │   └── model.go                  # Semua struct: Product, Order, Session, dll
│   ├── repository/                   # Lapisan database (PostgreSQL via sqlx)
│   │   ├── product_repository.go     # Search, reserve/deduct stock
│   │   ├── order_repository.go       # CRUD order, expire reservations
│   │   └── session_repository.go     # Upsert & reset session per user
│   ├── service/                      # Business logic
│   │   ├── intent_service.go         # Rule-based → Gemini AI fallback
│   │   ├── product_service.go        # Search dengan in-memory cache
│   │   ├── message_processor.go      # Orchestrator utama chatbot
│   │   ├── message_formater.go       # Template pesan WhatsApp (Indonesian)
│   │   ├── order_service.go          # Reserve → Confirm → Cancel flow
│   │   ├── messaging_service.go      # Kirim pesan via Fonnte API
│   │   └── telegram_service.go       # Kirim pesan via Telegram API
│   ├── scraper/
│   │   └── scraper.go                # Marketplace Scraper (Go-Rod)
│   ├── ai/
│   │   ├── gemini.go                 # Intent detection + image identification
│   │   └── http.go                   # HTTP helper untuk Gemini REST API
│   ├── cache/
│   │   └── memory.go                 # In-memory TTL cache (zero dependency)
│   ├── worker/
│   │   └── pool.go                   # Goroutine worker pool + panic recovery
│   ├── handler/
│   │   ├── webhook_handler.go        # Terima POST dari Fonnte, dispatch ke worker
│   │   └── telegram_webhook_handler.go# Terima POST dari Telegram
│   └── middleware/
│       └── middleware.go             # Logger, recovery, rate limiter per user
├── migrations/
│   └── 001_init.sql                  # Schema PostgreSQL + 10 produk seed data
├── .env.example                      # Template environment variables
├── go.mod
└── README.md
```

---

## API Endpoints

### 1. Webhook & Monitoring
| Method | Path | Deskripsi |
|--------|------|-----------|
| `POST` | `/webhook` | Endpoint utama — dipanggil Fonnte setiap ada pesan masuk |
| `POST` | `/webhook/telegram` | Webhook Telegram bot CS |
| `POST` | `/webhook/midtrans` | Webhook notifikasi pembayaran Midtrans |
| `GET` | `/health` | Health check / liveness probe |
| `GET` | `/metrics` | Stats worker queue (jumlah job pending) |
| `GET` | `/fonnte/test` | Cek koneksi & kirim pesan WhatsApp test |

### 2. Courier Map & Web App
| Method | Path | Deskripsi |
|--------|------|-----------|
| `GET` | `/api/v1/courier/map-view` | Peta interaktif Leaflet OSM multi-pin point rute kurir |
| `GET` | `/api/v1/courier/manifest-data` | Data JSON manifest & stop points pengantaran |

### 3. Deliveries & Scheduling (`/api/v1/deliveries`)
| Method | Path | Deskripsi |
|--------|------|-----------|
| `GET` | `/api/v1/deliveries/available-schedules?date=YYYY-MM-DD` | List slot pengantaran & kuota sisa untuk tanggal tertentu |
| `POST` | `/api/v1/deliveries/request` | Buat permintaan pengantaran pesanan + hitung ongkir GPS |
| `POST` | `/api/v1/deliveries/:id/approve` | Kurir mengonfirmasi jadwal pengantaran |
| `POST` | `/api/v1/deliveries/:id/reschedule-suggest` | Kurir mengajukan saran tanggal/slot baru ke customer |
| `POST` | `/api/v1/deliveries/:id/reschedule-accept` | Customer menyetujui saran perubahan jadwal dari kurir |

### Contoh Request Webhook (dari Fonnte)

Fonnte mengirim data dalam format **form-data** atau **JSON**. Bot ini mendukung keduanya.

**Field yang didukung:**
- `sender`: Nomor pengirim (628...)
- `message`: Isi pesan teks
- `name`: Nama tampilan WhatsApp
- `location`: Koordinat lokasi (jika kirim share location)
- `file`: URL file yang diterima (gambar/dokumen)
- `url`: Media URL (full-feature device)
- `filename`: Nama file (full-feature device)

```bash
curl -X POST http://localhost:8080/webhook \
  -d "sender=6281234567890" \
  -d "message=cari kampas rem" \
  -d "name=Budi"
```

### Response

```json
{ "status": "queued" }
```

### Health Check

```bash
curl http://localhost:8080/health
```
```json
{ "status": "ok", "time": "2024-01-15T10:00:00Z" }
```

### Metrics

```bash
curl http://localhost:8080/metrics
```
```json
{ "queue_length": 0 }
```

---

## Environment Variables

| Variable | Wajib | Default | Deskripsi |
|----------|-------|---------|-----------|
| `APP_PORT` | | `8080` | Port HTTP server |
| `APP_ENV` | | `development` | Set `production` untuk disable debug log |
| `APP_BASE_URL` | | `http://localhost:8080` | Domain publik (HTTPS) untuk tombol link WebApp Telegram |
| `DB_HOST` | | `localhost` | PostgreSQL host |
| `DB_PORT` | | `5432` | PostgreSQL port |
| `DB_USER` | | `postgres` | PostgreSQL user |
| `DB_PASS` | ✅ | — | PostgreSQL password |
| `DB_NAME` | | `autoparts_db` | Nama database |
| `FONNTE_TOKEN` | ✅ | — | Token dari dashboard Fonnte |
| `TELE_API` | | — | Token Bot 1: CS Chatbot Telegram |
| `TELEGRAM_NOTIFIER_BOT_TOKEN` | | — | Token Bot 2: Notifier & Channel Broadcaster |
| `TELEGRAM_COURIER_BOT_TOKEN` | | — | Token Bot 3: Courier Delivery Assistant |
| `COURIER_REMINDER_MODE` | | `daily` | Mode reminder kurir: `daily` (jam tertentu) atau `interval` (per menit/jam) |
| `COURIER_REMINDER_INTERVAL_MINUTES` | | `60` | Interval pengiriman reminder dalam menit (saat mode `interval`) |
| `COURIER_REMINDER_HOUR` | | `5` | Jam pengiriman reminder harian (0-23 WIB, saat mode `daily`) |
| `COURIER_REMINDER_MINUTE` | | `0` | Menit pengiriman reminder harian (0-59 WIB, saat mode `daily`) |
| `TELEGRAM_ORDER_CHANNEL_ID` | | — | Channel Telegram untuk broadcast order baru |
| `TELEGRAM_ERROR_CHANNEL_ID` | | — | Channel Telegram untuk error alerting |
| `FONNTE_API_URL` | | `https://api.fonnte.com/send` | Endpoint Fonnte (jangan diubah) |
| `GEMINI_API_KEY` | ✅ | — | Key dari Google AI Studio |
| `GEMINI_MODEL` | | `gemini-1.5-flash-latest` | Model Gemini yang dipakai |
| `GEMINI_TIMEOUT_SEC` | | `3` | Timeout AI dalam detik |
| `WORKER_POOL_SIZE` | | `10` | Jumlah goroutine worker paralel |
| `WORKER_QUEUE_SIZE` | | `100` | Kapasitas antrian job |
| `SESSION_TTL_MINUTES` | | `30` | Sesi user expired setelah N menit idle |
| `CACHE_PRODUCT_TTL_SECONDS` | | `60` | Hasil pencarian produk di-cache N detik |
| `CACHE_CLEANUP_INTERVAL_SECONDS` | | `120` | Seberapa sering cache di-sweep |
| `RATE_LIMIT_PER_SECOND` | | `5` | Maks pesan per detik per nomor |
| `RATE_LIMIT_BURST` | | `10` | Burst maksimal rate limiter |

---

## Performa

| Komponen | Latency | Keterangan |
|----------|---------|------------|
| Rule-based intent | ~0ms | Tidak ada network call |
| Cache hit (produk) | ~0.1ms | In-memory lookup |
| DB product search | ~5–20ms | Query LIKE dengan index |
| Gemini AI intent | ~500–2000ms | Hanya dipanggil jika rule gagal |
| Gemini image ID | ~1000–3000ms | Selalu AI, ada timeout 3 detik |
| **Total (rule path)** | **< 500ms** | |
| **Total (AI path)** | **< 4 detik** | |

- **Atomic stock reservation**: `UPDATE ... WHERE (stock - reserved) >= qty` → tidak ada oversell tanpa row lock penuh
- **In-memory cache**: mencegah DB query berulang untuk keyword yang sama dalam 60 detik
- **Goroutine worker pool**: webhook langsung return 200, pemrosesan berjalan paralel

---

## Checklist Produksi

- [ ] Set `APP_ENV=production` di environment
- [ ] Setup SSL/TLS — gunakan nginx atau Caddy sebagai reverse proxy
- [ ] Daftarkan domain + HTTPS agar webhook Fonnte & Telegram diterima
- [ ] Jalankan sebagai systemd service atau Docker container
- [ ] Setup log rotation (zap production mode sudah JSON)
- [ ] Naikkan `WORKER_POOL_SIZE` jika traffic tinggi (mulai dari 20)
- [ ] Pertimbangkan PostgreSQL read replica untuk query pencarian
- [ ] Tambahkan Prometheus exporter untuk monitoring
- [ ] Backup database secara berkala

### Contoh systemd service

```ini
# /etc/systemd/system/autoparts-bot.service
[Unit]
Description=WhatsApp AutoParts Chatbot
After=network.target mysql.service

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/autoparts
EnvironmentFile=/opt/autoparts/.env
ExecStart=/opt/autoparts/bin/autoparts
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable autoparts-bot
sudo systemctl start autoparts-bot
sudo systemctl status autoparts-bot
```

### Contoh nginx reverse proxy

```nginx
server {
    listen 443 ssl;
    server_name yourdomain.com;

    ssl_certificate     /etc/letsencrypt/live/yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/yourdomain.com/privkey.pem;

    location /webhook {
        proxy_pass         http://127.0.0.1:8080;
        proxy_set_header   Host $host;
        proxy_set_header   X-Real-IP $remote_addr;
        proxy_read_timeout 10s;
    }

    location /health {
        proxy_pass http://127.0.0.1:8080;
    }
}
```
