# Dokumen API & Postman Collection — NiagaGudang AutoParts

Dokumentasi ini berisi panduan penggunaan API untuk **Products**, **Categories**, **Users**, **Customers**, dan **Webhooks**.

File Postman Collection resmi yang dapat di-import langsung ke Postman berada di:
[postman_collection.json](file:///c:/Kuliah/smt4/NiagaGudang/postman_collection.json)

---

## 🚀 Cara Import ke Postman

1. Buka aplikasi **Postman**.
2. Klik tombol **Import** (di pojok kiri atas).
3. Pilih file [`postman_collection.json`](file:///c:/Kuliah/smt4/NiagaGudang/postman_collection.json).
4. Pastikan Environment Variable `base_url` diatur ke `http://localhost:8080`.

---

## 📦 Endpoint Reference

### 1. Products (`/api/v1/products`)

| Method | Endpoint | Deskripsi |
|---|---|---|
| `GET` | `/api/v1/products` | Ambil semua produk aktif. Parameter opsional: `q`, `category_id`, `stock_status`, `is_active`, `low_stock_priority` (true/false) |
| `GET` | `/api/v1/products?low_stock_priority=true` | Ambil produk memprioritaskan barang dengan stok menipis/habis di urutan teratas (Data Master / Alert Restock) |
| `GET` | `/api/v1/products?low_stock_priority=false` | Ambil produk memprioritaskan barang yang masih tersedia stoknya di urutan teratas (POS) |
| `GET` | `/api/v1/products?q=kampas` | Cari produk fuzzy search by name/sku/category |
| `GET` | `/api/v1/products/:id` | Ambil detail produk berdasarkan ID & referensi harga pasar |
| `POST` | `/api/v1/products` | Tambah produk baru |
| `PUT` | `/api/v1/products/:id` | Update data produk |
| `DELETE` | `/api/v1/products/:id` | Soft delete produk (`is_active = false`) |

#### Contoh Payload `POST /api/v1/products`:
```json
{
  "sku": "KR-HB-003",
  "name": "Kampas Rem Honda Vario Depan",
  "category_id": 1,
  "description": "Kampas rem ori AHM Vario",
  "stock": 25,
  "reserved": 0,
  "location": "Rak A1-B3",
  "price": 40000,
  "unit": "pcs",
  "is_active": true
}
```

---

### 2. Categories (`/api/v1/categories`)

| Method | Endpoint | Deskripsi |
|---|---|---|
| `GET` | `/api/v1/categories` | Ambil semua kategori |
| `GET` | `/api/v1/categories/:id` | Ambil detail kategori by ID |
| `POST` | `/api/v1/categories` | Tambah kategori baru |
| `PUT` | `/api/v1/categories/:id` | Update data kategori |
| `DELETE` | `/api/v1/categories/:id` | Hapus kategori |

#### Contoh Payload `POST /api/v1/categories`:
```json
{
  "name": "Aksesoris",
  "slug": "aksesoris",
  "description": "Kategori suku cadang dan aksesoris kendaraan"
}
```

---

### 3. Users (`/api/v1/users`)

| Method | Endpoint | Deskripsi |
|---|---|---|
| `GET` | `/api/v1/users` | Ambil semua pengguna |
| `GET` | `/api/v1/users/:id` | Ambil detail pengguna by ID |
| `POST` | `/api/v1/users` | Tambah pengguna baru (password otomatis di-hash `bcrypt`) |
| `PUT` | `/api/v1/users/:id` | Update data pengguna |
| `DELETE` | `/api/v1/users/:id` | Hapus pengguna |

#### Contoh Payload `POST /api/v1/users`:
```json
{
  "username": "budi_staff",
  "email": "budi@niagagudang.com",
  "password": "rahasia123",
  "name": "Budi Santoso",
  "role": "staff",
  "phone": "081234567890"
}
```

---

### 4. Customers (`/api/v1/customers`)

| Method | Endpoint | Deskripsi |
|---|---|---|
| `GET` | `/api/v1/customers` | Ambil semua pelanggan |
| `GET` | `/api/v1/customers/:id` | Ambil detail pelanggan by ID |
| `POST` | `/api/v1/customers` | Tambah pelanggan baru |
| `PUT` | `/api/v1/customers/:id` | Update data pelanggan |
| `DELETE` | `/api/v1/customers/:id` | Hapus pelanggan |

#### Contoh Payload `POST /api/v1/customers`:
```json
{
  "name": "Ahmad Yani",
  "phone_number": "6285712345678",
  "email": "ahmad@gmail.com",
  "address": "Jl. Merdeka No. 45 Jakarta",
  "notes": "Pelanggan langganan oli"
}
```

---

### 5. Orders (`/api/v1/orders`)

| Method | Endpoint | Deskripsi |
|---|---|---|
| `GET` | `/api/v1/orders` | Ambil daftar pesanan (Filter: `user_id`, `status`, `q`, `start_date`, `end_date`, `date`, `page`, `limit`) |
| `GET` | `/api/v1/orders?start_date=2026-08-01&end_date=2026-08-11` | Ambil daftar pesanan dalam rentang tanggal tertentu |
| `GET` | `/api/v1/orders?date=2026-08-11` | Ambil daftar pesanan khusus pada 1 tanggal spesifik |
| `GET` | `/api/v1/orders?user_id=1` | Ambil daftar pesanan khusus milik User ID / Customer tertentu |
| `GET` | `/api/v1/orders/:id` | Ambil detail pesanan berdasarkan ID |
| `POST` | `/api/v1/orders` | Buat pesanan baru (POS / Web Checkout) |
| `DELETE` | `/api/v1/orders/:id` | Hapus pesanan secara permanen (**Khusus status `cancelled`**) |

#### Contoh Respon `GET /api/v1/orders?user_id=1`:
```json
{
  "data": [
    {
      "id": 1,
      "order_number": "APT-20260805-A3F9",
      "user_id": 1,
      "total_price": 75000,
      "amount_paid": 100000,
      "change_amount": 25000,
      "status": "paid",
      "source": "pos",
      "payment_method": "cash",
      "notes": "Pembelian via Kasir POS",
      "expires_at": null,
      "created_at": "2026-08-05T10:30:00+07:00",
      "updated_at": "2026-08-05T10:30:00+07:00",
      "items": [
        {
          "id": 1,
          "order_id": 1,
          "product_id": 1,
          "product_name": "Kampas Rem Honda Vario Depan",
          "quantity": 1,
          "unit_price": 40000,
          "subtotal": 40000,
          "created_at": "2026-08-05T10:30:00+07:00"
        }
      ]
    }
  ],
  "meta": {
    "page": 1,
    "limit": 10,
    "total": 1
  }
}
```

---

### 6. Payments & Midtrans Integration (`/api/v1/payments`)

| Method | Endpoint | Deskripsi |
|---|---|---|
| `GET` | `/api/v1/payments/config` | Mengambil `client_key` & status `is_production` Midtrans |
| `POST` | `/api/v1/payments/snap-token` | Membuat Snap Token & Redirect URL Midtrans berdasarkan `order_id` |
| `POST` | `/api/v1/payments/midtrans-notification` | Webhook callback status pembayaran dari Midtrans |
| `POST` | `/webhook/midtrans` | Alias endpoint webhook callback status pembayaran dari Midtrans |

#### Contoh Payload `POST /api/v1/payments/snap-token`:
```json
{
  "order_id": 1
}
```

#### Contoh Payload Webhook Notification (`POST /webhook/midtrans`):
```json
{
  "order_id": "APT-20260805-A3F9",
  "status_code": "200",
  "gross_amount": "75000.00",
  "transaction_status": "settlement",
  "signature_key": "{{calc_signature_key}}",
  "payment_type": "bank_transfer",
  "transaction_time": "2026-08-06 15:00:00"
}
```

> **Tips di Postman:** Pada request `Midtrans Notification Webhook (Settlement / Paid)`, terdapat **Pre-request Script** otomatis yang menghitung `signature_key` menggunakan SHA512 (`SHA512(order_id + status_code + gross_amount + server_key)`) sehingga verifikasi pembayaran Midtrans di backend selalu berhasil tanpa error signature verification failed!

### 6. Webhooks

| Method | Endpoint | Deskripsi |
|---|---|---|
| `POST` | `/webhook` | Simulasi webhook masuk dari Fonnte (WhatsApp) |
| `POST` | `/telegram/webhook` | Simulasi webhook masuk dari Telegram Bot |

#### Contoh Payload `POST /webhook` (WhatsApp Fonnte):
```json
{
  "sender": "628123456789",
  "message": "stok kampas rem beat"
}
```

### 7. Dashboard (`/api/v1/dashboard`)

| Method | Endpoint | Deskripsi |
|---|---|---|
| `GET` | `/api/v1/dashboard` | Ambil data statistik ringkasan dashboard, 5 order terbaru, dan 10 produk stok menipis |
| `GET` | `/api/v1/dashboard/summary` | Alias endpoint untuk ringkasan dashboard |

#### Contoh Respon `GET /api/v1/dashboard`:
```json
{
  "data": {
    "summary": {
      "total_products": 45,
      "total_categories": 6,
      "total_customers": 12,
      "total_orders": 28,
      "pending_orders": 3,
      "paid_orders": 22,
      "cancelled_orders": 3,
      "total_revenue": 3450000.00,
      "low_stock_products": 4,
      "out_of_stock_products": 1
    },
    "recent_orders": [
      {
        "id": 28,
        "order_number": "ORD-20260807-001",
        "customer_name": "Budi Santoso",
        "total_price": 150000.00,
        "status": "paid",
        "created_at": "2026-08-07T14:30:00Z"
      }
    ],
    "low_stock_items": [
      {
        "id": 12,
        "sku": "KR-HB-001",
        "name": "Kampas Rem Depan Honda Beat",
        "category": "Sistem Pengereman",
        "stock": 3,
        "reserved": 1,
        "available": 2,
        "minimum_stock": 5,
        "price": 35000.00
      }
    ]
  }
}
```

### 8. Reports & CSV Export (`/api/v1/reports`)

| Method | Endpoint | Deskripsi |
|---|---|---|
| `GET` | `/api/v1/reports/sales` | Ambil laporan transaksi penjualan (Filter: `start_date`, `end_date`, `status`, `page`, `limit`) |
| `GET` | `/api/v1/reports/export/pdf` | Download berkas **PDF Laporan Penjualan** (Filter: `start_date`, `end_date`, `status`) |
| `GET` | `/api/v1/reports/export/excel` | Download berkas **Excel (.xlsx) Laporan Penjualan** (Filter: `start_date`, `end_date`, `status`) |
| `GET` | `/api/v1/reports/sales/export` | Download berkas **CSV Laporan Penjualan** (Filter: `start_date`, `end_date`, `status`) |
| `GET` | `/api/v1/reports/sales/export/pdf` | Download berkas **PDF Laporan Penjualan** (Filter: `start_date`, `end_date`, `status`) |
| `GET` | `/api/v1/reports/sales/export/excel` | Download berkas **Excel (.xlsx) Laporan Penjualan** (Filter: `start_date`, `end_date`, `status`) |
| `GET` | `/api/v1/reports/stock/export` | Download berkas **CSV Laporan Stok Produk Gudang** (Filter: `category_id`, `low_stock_only=true`) |
| `GET` | `/api/v1/reports/stock/export/pdf` | Download berkas **PDF Laporan Stok Produk Gudang** (Filter: `category_id`, `low_stock_only=true`) |
| `GET` | `/api/v1/reports/stock/export/excel` | Download berkas **Excel (.xlsx) Laporan Stok Produk Gudang** (Filter: `category_id`, `low_stock_only=true`) |

#### Contoh Respon `GET /api/v1/reports/sales?start_date=2026-08-01&end_date=2026-08-07`:
```json
{
  "data": {
    "summary": {
      "total_orders": 15,
      "total_revenue": 2250000.00,
      "total_items": 34
    },
    "orders": [
      {
        "id": 28,
        "order_number": "ORD-20260807-001",
        "customer_name": "Budi Santoso",
        "total_price": 150000.00,
        "status": "paid",
        "payment_method": "qris",
        "source": "pos",
        "item_count": 2,
        "created_at": "2026-08-07T14:30:00Z"
      }
    ]
  },
  "meta": {
    "page": 1,
    "limit": 10,
    "total": 15,
    "total_pages": 2
  }
}
```

> **Catatan Fitur Export CSV:** Endpoint `/sales/export` dan `/stock/export` menyertakan Header UTF-8 BOM (`\uFEFF`) serta Content-Disposition `attachment` sehingga file CSV yang terunduh langsung rapi dan dapat dibuka secara otomatis di Microsoft Excel dan Google Sheets tanpa masalah karakter/tata letak.


