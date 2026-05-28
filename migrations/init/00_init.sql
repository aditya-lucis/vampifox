-- VampiFox — Database Initialization
-- Dijalankan otomatis saat container PostgreSQL pertama kali dibuat.
-- Untuk driver lain, migration ini dijalankan manual via foxctl.

-- ── Extensions ───────────────────────────────────────────────────
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- ── Schema sistem ─────────────────────────────────────────────────
CREATE SCHEMA IF NOT EXISTS vfx_system;

-- Set search_path default untuk session
ALTER DATABASE vampifox_main SET search_path TO vfx_system, public;

-- ── Tabel tenants ─────────────────────────────────────────────────
-- Disimpan di vfx_system — data global, bukan per-tenant.
CREATE TABLE IF NOT EXISTS vfx_system.tenants (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug         VARCHAR(100)  NOT NULL,
    name         VARCHAR(255)  NOT NULL,
    domain       VARCHAR(255),
    plan         VARCHAR(50)   NOT NULL DEFAULT 'starter',
    status       VARCHAR(50)   NOT NULL DEFAULT 'active',
    schema_name  VARCHAR(100)  NOT NULL,
    max_users    INTEGER       NOT NULL DEFAULT 10,
    storage_gb   INTEGER       NOT NULL DEFAULT 5,
    settings     JSONB                  DEFAULT '{}',
    created_at   TIMESTAMPTZ            DEFAULT NOW(),
    updated_at   TIMESTAMPTZ            DEFAULT NOW(),
    suspended_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ,

    CONSTRAINT tenants_slug_key   UNIQUE (slug),
    CONSTRAINT tenants_domain_key UNIQUE (domain)
);

CREATE INDEX IF NOT EXISTS idx_tenants_slug   ON vfx_system.tenants(slug);
CREATE INDEX IF NOT EXISTS idx_tenants_domain ON vfx_system.tenants(domain);
CREATE INDEX IF NOT EXISTS idx_tenants_status ON vfx_system.tenants(status);

COMMENT ON TABLE vfx_system.tenants IS
    'Daftar tenant VampiFox — setiap baris adalah satu wilayah kekuasaan';

-- ── Fungsi: buat schema tenant ────────────────────────────────────
-- Dipanggil saat tenant baru di-provision
CREATE OR REPLACE FUNCTION vfx_system.provision_tenant_schema(p_schema_name TEXT)
RETURNS VOID AS $$
BEGIN
    EXECUTE format('CREATE SCHEMA IF NOT EXISTS %I', p_schema_name);
    EXECUTE format('SET search_path TO %I, public', p_schema_name);
END;
$$ LANGUAGE plpgsql;

-- ── Tabel users (template, dibuat di setiap schema tenant) ────────
-- Ini adalah definisi template — foxctl akan menjalankan ini
-- di setiap schema tenant saat modul di-boot.
-- Di sini hanya sebagai dokumentasi struktur.
COMMENT ON SCHEMA vfx_system IS
    'Schema sistem VampiFox — berisi data global lintas tenant';

SELECT 'VampiFox database initialized. Kerajaan siap dibangun. 🦊🧛' AS status;
