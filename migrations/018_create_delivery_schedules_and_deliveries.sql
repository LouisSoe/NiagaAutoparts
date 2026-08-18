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
INSERT INTO delivery_schedules (day_of_week, slot_name, start_time, end_time, max_capacity) VALUES
('Senin', 'Slot Pagi (09:00 - 12:00)', '09:00:00', '12:00:00', 5),
('Senin', 'Slot Siang (13:00 - 16:00)', '13:00:00', '16:00:00', 5),
('Selasa', 'Slot Pagi (09:00 - 12:00)', '09:00:00', '12:00:00', 5),
('Selasa', 'Slot Siang (13:00 - 16:00)', '13:00:00', '16:00:00', 5),
('Rabu', 'Slot Pagi (09:00 - 12:00)', '09:00:00', '12:00:00', 5),
('Rabu', 'Slot Siang (13:00 - 16:00)', '13:00:00', '16:00:00', 5),
('Kamis', 'Slot Pagi (09:00 - 12:00)', '09:00:00', '12:00:00', 5),
('Kamis', 'Slot Siang (13:00 - 16:00)', '13:00:00', '16:00:00', 5),
('Jumat', 'Slot Pagi (09:00 - 11:30)', '09:00:00', '11:30:00', 4),
('Jumat', 'Slot Siang (13:30 - 16:30)', '13:30:00', '16:30:00', 4),
('Sabtu', 'Slot Pagi (09:00 - 12:00)', '09:00:00', '12:00:00', 5),
('Sabtu', 'Slot Siang (13:00 - 15:00)', '13:00:00', '15:00:00', 3)
ON CONFLICT DO NOTHING;

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
