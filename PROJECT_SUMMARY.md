# PROJECT SUMMARY — WhatsApp AutoParts Chatbot
> Dokumen ini dibuat untuk melanjutkan pengembangan di session berikutnya.
> Paste dokumen ini di awal percakapan baru bersama Claude.

---

## 🎯 Ringkasan Project

**Nama project:** `niaga-autoparts`  
**Module Go:** `github.com/louissoe/niaga-autoparts`  
**Bahasa:** Go 1.24.0  
**Tujuan:** Chatbot WhatsApp & Telegram untuk toko suku cadang otomotif — pencarian produk, pemesanan dengan reservasi stok, identifikasi foto via AI  

**Stack:**
- Messaging Gateway: **Fonnte** (WhatsApp Webhook POST) & **Telegram Bot API**
- AI: **Google Gemini Flash Lite** (intent detection fallback + image identification)
- Database: **PostgreSQL 12+** via `sqlx` (dengan ekstensi `pg_trgm` untuk fuzzy search)
- HTTP Framework: **Gin**
- Logger: **Zap**
- Cache: **In-memory native Go** (`sync.RWMutex` + background eviction goroutine) — **TANPA Redis**
- Scraper: **Go-Rod** (Headless Browser)

---

## 📁 Struktur File

```
niaga-autoparts/
├── cmd/main.go                               # Entry point, wiring semua komponen
├── go.mod                                    # Dependencies (tanpa redis)
├── .env.example                              # Template env vars
├── README.md                                 # Dokumentasi lengkap + cara pakai
├── migrations/001_init.sql                   # Schema PostgreSQL + 10 produk seed
└── internal/
    ├── config/config.go                      # Load env → struct Config
    ├── model/                              
    ├── cache/memory.go                       # In-memory TTL cache
    ├── ai/
    │   ├── gemini.go                         # Gemini REST API (intent + image)
    │   └── http.go                           # HTTP helper reusable
    ├── repository/
    │   ├── product_repository.go             # Search, reserve/deduct stock
    │   ├── order_repository.go               # CRUD order, expire reservations
    │   └── session_repository.go             # Upsert session per user
    ├── service/
    │   ├── intent_service.go                 # Rule-based → AI fallback
    │   ├── product_service.go                # Search + in-memory cache layer
    │   ├── message_processor.go              # Orchestrator utama chatbot
    │   ├── message_formater.go               # Template pesan (Indonesian)
    │   ├── order_service.go                  # Reserve → Confirm → Cancel
    │   ├── messaging_service.go              # Kirim pesan via Fonnte API
    │   └── telegram_service.go               # Kirim pesan via Telegram API
    ├── scraper/                              # Marketplace Scraper (Go-Rod)
    ├── utils/                                # Utils (Spell Check, dll)
    ├── worker/pool.go                        # Goroutine worker pool
    ├── handler/
    │   ├── webhook_handler.go                # Terima webhook Fonnte
    │   └── telegram_webhook_handler.go       # Terima webhook Telegram
    └── middleware/middleware.go              # Logger, recovery, rate limiter
```

---

## 🧩 Arsitektur & Flow

```
Fonnte POST /webhook
       ↓
  WebhookHandler
    1. Parse payload (form-encoded)
    2. Goroutine → kirim "⏳ Sedang diproses..." (<1 detik)
    3. Dispatch job ke Worker Pool
    4. Return HTTP 200 langsung
       ↓
  Worker Pool (N goroutines, default 10)
    - Channel buffered sebagai queue
    - Panic recovery per job
       ↓
  MessageProcessor.Process()
    1. GetOrCreate session dari DB
    2. Image? → AI identify → reply
    3. IntentService.Detect()
       - Rule-based keyword matching (0ms)
       - Jika tidak cocok → Gemini API (timeout 3 detik)
    4. Dispatch ke handler sesuai intent
    5. Save session
    6. Send reply via MessagingService
```

**Session State Machine:**
```
idle → searching → awaiting_qty → awaiting_confirm → idle (selesai/batal)
```

**Order Flow (2-phase):**
```
pesan produk N → ReserveStock (atomic UPDATE) → kirim konfirmasi
     ↓ user balas "ya"
ConfirmOrder() → TX: DeductStock + UpdateStatus=paid
     ↓ user balas "batal" atau timeout 15 menit
CancelOrder() → ReleaseReservation
```

---

## 🗄️ Database Schema (4 tabel)

### `products`
| Kolom | Type | Keterangan |
|-------|------|------------|
| id | BIGINT PK | |
| sku | VARCHAR(50) UNIQUE | |
| name | VARCHAR(255) | FULLTEXT index |
| category | VARCHAR(100) | |
| stock | INT | Total stok |
| reserved | INT | Stok yang di-hold order pending |
| location | VARCHAR(100) | Posisi rak gudang |
| price | DECIMAL(15,2) | |
| unit | VARCHAR(20) | pcs, set, botol, dll |
| is_active | TINYINT(1) | Soft delete |

**Atomic reserve:** `UPDATE products SET reserved=reserved+N WHERE (stock-reserved)>=N`

### `orders`
| Status | Arti |
|--------|------|
| `pending` | Dibuat, belum reservasi |
| `reserved` | Stok direservasi, tunggu konfirmasi |
| `paid` | Konfirmasi diterima, stok dipotong |
| `cancelled` | Dibatalkan, stok dilepas |

Kolom penting: `expires_at` (reservasi expired setelah 15 menit)

### `sessions`
UNIQUE per `phone_number`. Kolom state machine:
- `state`: idle / searching / awaiting_qty / awaiting_confirm / ordering
- `last_product_id`, `last_product_name`: konteks produk terakhir
- `pending_order_id`: order yang menunggu konfirmasi
- `expires_at`: TTL 30 menit (default), di-extend setiap pesan

### `price_references`
Harga marketplace per produk (Tokopedia, Shopee, dll). Ditampilkan saat detail produk.

---

## 🔑 Key Design Decisions

### 1. Rule-based first, AI second
`IntentService.Detect()` cek keyword Indonesia dulu (greet, cari, harga, pesan, ya, batal, dll). AI dipanggil **hanya** jika tidak ada keyword yang cocok. Ini membuat 80%+ pesan diproses tanpa network call ke Gemini.

### 2. In-memory cache (bukan Redis)
`cache/memory.go` — `sync.RWMutex` + `map[string]entry{value, expiresAt}`. Background goroutine sweep expired entries setiap 2 menit. API identik dengan wrapper Redis lama sehingga mudah diganti jika suatu saat butuh distributed cache.

```go
// Penggunaan di product_service.go
cacheKey := "product_search:" + strings.ToLower(query)
if raw, ok := s.cache.Get(ctx, cacheKey); ok { ... }  // cache hit
// cache miss → DB → s.cache.Set(ctx, cacheKey, json, ttl)
```

### 3. Goroutine worker pool
Webhook return 200 dalam < 1 detik. Semua heavy lifting (DB, AI) jalan di goroutine pool. Back-pressure: jika queue penuh (100 job), kirim pesan "sistem sibuk" ke user.

### 4. Session state-aware intent
`detectByState()` dipanggil sebelum keyword matching. Jika session di state `awaiting_qty`, angka apapun langsung diinterpretasi sebagai quantity — tidak perlu AI.

---

## 📦 Dependencies (go.mod)

```
github.com/gin-gonic/gin v1.10.0          # HTTP framework
github.com/lib/pq v1.12.3                 # PostgreSQL driver
github.com/jmoiron/sqlx v1.4.0            # SQL helper
github.com/joho/godotenv v1.5.1           # .env loader
github.com/go-telegram-bot-api/telegram-bot-api/v5 # Telegram API
github.com/go-rod/rod v0.116.2            # Headless Browser for Scraper
go.uber.org/zap v1.27.0                   # Logger
golang.org/x/time v0.5.0                  # Rate limiter
```

> ❌ **Tidak ada redis/go-redis** — sudah dihapus, diganti in-memory cache

---

## ⚙️ Environment Variables Penting

```env
# Wajib diisi
FONNTE_TOKEN=...
TELE_API=...
GEMINI_API_KEY=...
DB_PASS=...

# Cache tuning
CACHE_PRODUCT_TTL_SECONDS=60       # default 60 detik
CACHE_CLEANUP_INTERVAL_SECONDS=120 # default 2 menit

# Performance tuning
WORKER_POOL_SIZE=10
WORKER_QUEUE_SIZE=100
SESSION_TTL_MINUTES=30
GEMINI_TIMEOUT_SEC=3
RATE_LIMIT_PER_SECOND=5
```

---

## ✅ Status Implementasi

| Fitur | Status |
|-------|--------|
| Webhook handler (Fonnte & Telegram) | ✅ Done |
| Worker pool (goroutine) | ✅ Done |
| Session management (DB) | ✅ Done |
| Intent detection rule-based | ✅ Done |
| Intent detection AI fallback | ✅ Done |
| Product search + cache | ✅ Done |
| Marketplace price comparison | ✅ Done |
| Order reservation (atomic) | ✅ Done |
| Order confirm (TX) | ✅ Done |
| Order cancel + stock release | ✅ Done |
| Order expiry (background ticker) | ✅ Done |
| Image identification (Gemini) | ✅ Done |
| In-memory cache (tanpa Redis) | ✅ Done |
| Rate limiter per user | ✅ Done |
| Middleware (logger, recovery) | ✅ Done |
| Database schema + seed data | ✅ Done |
| README + cara pakai | ✅ Done |
| Marketplace price auto-fetch | ✅ Done (Go-Rod Scraper) |
| Unit tests | ❌ Belum |
| Admin dashboard | ❌ Belum |
| Prometheus metrics | ❌ Belum |

---

## 🔧 Yang Bisa Dilanjutkan

Berikut hal-hal yang belum diimplementasikan dan bisa jadi next step:

1. **Unit tests** — `service/intent_service_test.go`, `repository/product_repository_test.go`, mock untuk AI
2. **Admin endpoint** — tambah/edit/hapus produk via REST API (GET/POST/PUT/DELETE `/api/products`)
3. **Prometheus metrics** — expose `/metrics` dengan gauge worker queue, counter pesan per intent, histogram latency
4. **Broadcast/blast pesan** — endpoint untuk admin kirim notifikasi ke semua pelanggan via Fonnte / Telegram
5. **Docker + docker-compose** — containerize app + PostgreSQL untuk deployment lebih mudah
6. **Multi-device Fonnte** — support lebih dari 1 nomor WA bot
8. **Payment gateway integration** — Midtrans/Xendit untuk konfirmasi pembayaran otomatis

---

## 💬 Prompt untuk Melanjutkan

Tempel teks ini di awal session baru:

```
Saya melanjutkan pengembangan project WhatsApp AutoParts Chatbot.

Tech stack: Go 1.24.0, Gin, PostgreSQL (sqlx + pg_trgm), Fonnte (WhatsApp gateway), Telegram Bot API,
Gemini Flash Lite (AI), in-memory cache native Go (tanpa Redis), Go-Rod (Scraper).

Module: github.com/louissoe/niaga-autoparts
Clean architecture: handler → service → repository → model
Worker pool goroutine untuk async processing.
In-memory cache di internal/cache/memory.go (sync.RWMutex + TTL).
Session state machine per user di tabel sessions.
2-phase order: reserve → confirm dengan atomic stock operation.

Yang sudah selesai: webhook, worker pool, intent detection (rule-based + AI fallback),
product search + cache, order flow lengkap, image identification, session management,
rate limiter, middleware, schema DB + seed data, README lengkap.

Yang ingin saya kerjakan selanjutnya: [TULIS DI SINI]
```
