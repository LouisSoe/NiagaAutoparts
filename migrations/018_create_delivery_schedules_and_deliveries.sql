-- 018_create_delivery_schedules_and_deliveries.sql

-- 1. Tambahkan kolom latitude dan longitude pada tabel customers jika belum ada
ALTER TABLE customers 
ADD COLUMN IF NOT EXISTS latitude DOUBLE PRECISION,
ADD COLUMN IF NOT EXISTS longitude DOUBLE PRECISION;

-- 2. Buat tabel master delivery_schedules (Hari, Range Jam, Kapasitas)
CREATE TABLE IF NOT EXISTS delivery_schedules (
    id SERIAL PRIMARY KEY,
    day_of_week VARCHAR(20) NOT NULL,
    slot_name VARCHAR(100) NOT NULL,
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    max_capacity INT NOT NULL DEFAULT 5,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Seed default schedules (Senin - Sabtu, Pagi & Siang)
INSERT INTO "delivery_schedules" ("id", "day_of_week", "slot_name", "start_time", "end_time", "max_capacity", "is_active", "created_at", "updated_at") VALUES (5, 'wednesday', 'Slot Pagi (08:00 - 09:30)', '08:00:00', '09:30:00', 10, 't', '2026-08-15 23:06:03.056613+07', '2026-08-18 08:57:52.334422+07');
INSERT INTO "delivery_schedules" ("id", "day_of_week", "slot_name", "start_time", "end_time", "max_capacity", "is_active", "created_at", "updated_at") VALUES (1, 'monday', 'Slot Pagi (09:00 - 12:00)', '09:00:00', '12:00:00', 5, 't', '2026-08-15 23:06:03.056613+07', '2026-08-18 08:58:38.239023+07');
INSERT INTO "delivery_schedules" ("id", "day_of_week", "slot_name", "start_time", "end_time", "max_capacity", "is_active", "created_at", "updated_at") VALUES (2, 'monday', 'Slot Siang (13:00 - 16:00)', '13:00:00', '16:00:00', 5, 't', '2026-08-15 23:06:03.056613+07', '2026-08-18 08:58:43.71154+07');
INSERT INTO "delivery_schedules" ("id", "day_of_week", "slot_name", "start_time", "end_time", "max_capacity", "is_active", "created_at", "updated_at") VALUES (3, 'tuesday', 'Slot Pagi (09:00 - 12:00)', '09:00:00', '12:00:00', 5, 't', '2026-08-15 23:06:03.056613+07', '2026-08-18 08:59:29.652964+07');
INSERT INTO "delivery_schedules" ("id", "day_of_week", "slot_name", "start_time", "end_time", "max_capacity", "is_active", "created_at", "updated_at") VALUES (4, 'tuesday', 'Slot Siang (13:00 - 16:00)', '13:00:00', '16:00:00', 5, 't', '2026-08-15 23:06:03.056613+07', '2026-08-18 08:59:38.915797+07');
INSERT INTO "delivery_schedules" ("id", "day_of_week", "slot_name", "start_time", "end_time", "max_capacity", "is_active", "created_at", "updated_at") VALUES (7, 'thursday', 'Slot Pagi (09:00 - 12:00)', '09:00:00', '12:00:00', 5, 't', '2026-08-15 23:06:03.056613+07', '2026-08-18 08:59:46.049719+07');
INSERT INTO "delivery_schedules" ("id", "day_of_week", "slot_name", "start_time", "end_time", "max_capacity", "is_active", "created_at", "updated_at") VALUES (8, 'thursday', 'Slot Siang (13:00 - 16:00)', '13:00:00', '16:00:00', 5, 't', '2026-08-15 23:06:03.056613+07', '2026-08-18 08:59:52.14271+07');
INSERT INTO "delivery_schedules" ("id", "day_of_week", "slot_name", "start_time", "end_time", "max_capacity", "is_active", "created_at", "updated_at") VALUES (9, 'friday', 'Slot Pagi (09:00 - 11:30)', '09:00:00', '11:30:00', 4, 't', '2026-08-15 23:06:03.056613+07', '2026-08-18 08:59:59.789046+07');
INSERT INTO "delivery_schedules" ("id", "day_of_week", "slot_name", "start_time", "end_time", "max_capacity", "is_active", "created_at", "updated_at") VALUES (10, 'friday', 'Slot Siang (13:30 - 16:30)', '13:30:00', '16:30:00', 4, 't', '2026-08-15 23:06:03.056613+07', '2026-08-18 09:00:07.149417+07');
INSERT INTO "delivery_schedules" ("id", "day_of_week", "slot_name", "start_time", "end_time", "max_capacity", "is_active", "created_at", "updated_at") VALUES (11, 'saturday', 'Slot Pagi (09:00 - 12:00)', '09:00:00', '12:00:00', 5, 't', '2026-08-15 23:06:03.056613+07', '2026-08-18 09:00:14.413162+07');
INSERT INTO "delivery_schedules" ("id", "day_of_week", "slot_name", "start_time", "end_time", "max_capacity", "is_active", "created_at", "updated_at") VALUES (12, 'saturday', 'Slot Siang (13:00 - 15:00)', '13:00:00', '15:00:00', 3, 't', '2026-08-15 23:06:03.056613+07', '2026-08-18 09:00:21.796847+07');
INSERT INTO "delivery_schedules" ("id", "day_of_week", "slot_name", "start_time", "end_time", "max_capacity", "is_active", "created_at", "updated_at") VALUES (14, 'wednesday', 'Slot Sore (16.00 - 17.30)', '16:00:00', '17:30:00', 10, 't', '2026-08-18 09:01:53.416253+07', '2026-08-18 09:01:53.416253+07');
INSERT INTO "delivery_schedules" ("id", "day_of_week", "slot_name", "start_time", "end_time", "max_capacity", "is_active", "created_at", "updated_at") VALUES (13, 'wednesday', 'Slot Siang Batch 2 (13:00 - 14.30)', '13:00:00', '14:30:00', 10, 't', '2026-08-18 09:01:16.280931+07', '2026-08-18 09:02:08.06629+07');
INSERT INTO "delivery_schedules" ("id", "day_of_week", "slot_name", "start_time", "end_time", "max_capacity", "is_active", "created_at", "updated_at") VALUES (6, 'wednesday', 'Slot Siang Batch 1 (11:00 - 12:30)', '13:00:00', '16:00:00', 10, 't', '2026-08-15 23:06:03.056613+07', '2026-08-18 09:02:25.015423+07');

-- 3. Buat tabel deliveries
CREATE TABLE IF NOT EXISTS deliveries (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    customer_id BIGINT REFERENCES customers(id) ON DELETE SET NULL,
    schedule_id BIGINT REFERENCES delivery_schedules(id) ON DELETE RESTRICT,
    courier_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    
    delivery_date DATE NOT NULL,
    status VARCHAR(35) NOT NULL DEFAULT 'waiting_courier_approval',
    
    shipping_cost NUMERIC(15, 2) DEFAULT 0,
    distance_km DOUBLE PRECISION DEFAULT 0,
    
    suggested_schedule_id BIGINT REFERENCES delivery_schedules(id),
    suggested_date DATE,
    rejection_reason TEXT,
    
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_deliveries_order_id ON deliveries(order_id);
CREATE INDEX IF NOT EXISTS idx_deliveries_customer_id ON deliveries(customer_id);
CREATE INDEX IF NOT EXISTS idx_deliveries_date_schedule ON deliveries(delivery_date, schedule_id);
CREATE INDEX IF NOT EXISTS idx_deliveries_status ON deliveries(status);
