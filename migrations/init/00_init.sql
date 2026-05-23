-- VampiFox — Database Initialization
-- Dijalankan otomatis saat container PostgreSQL pertama kali dibuat

-- Extension yang dibutuhkan
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";  -- untuk full-text search

-- Schema utama untuk sistem multi-tenant
-- Setiap tenant akan punya schema sendiri: vfx_{slug}
CREATE SCHEMA IF NOT EXISTS vfx_system;

-- Set search path default
ALTER DATABASE vampifox_main SET search_path TO vfx_system, public;

-- Tabel tenants (disimpan di schema system, bukan per-tenant)
CREATE TABLE IF NOT EXISTS vfx_system.tenants (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        VARCHAR(100) UNIQUE NOT NULL,
    name        VARCHAR(255) NOT NULL,
    domain      VARCHAR(255) UNIQUE,
    plan        VARCHAR(50) NOT NULL DEFAULT 'starter',
    status      VARCHAR(50) NOT NULL DEFAULT 'active',
    schema_name VARCHAR(100) NOT NULL,
    max_users   INT NOT NULL DEFAULT 10,
    storage_gb  INT NOT NULL DEFAULT 5,
    settings    JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    suspended_at TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ
);

COMMENT ON TABLE vfx_system.tenants IS 
    'Daftar tenant VampiFox — setiap tenant adalah wilayah kekuasaan';

SELECT 'VampiFox database initialized. Kerajaan siap dibangun. 🦊🧛' AS status;
