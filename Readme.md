<div align="center">

# 🦊🧛 VampiFox ERP Framework

**"The Night Never Sleeps, The Fox Never Rests"**

ERP Framework modern berbasis **Golang** — cepat, modular, dan multi-tenant.  
Terinspirasi dari ERPNext & Odoo, dibangun dengan karakter sendiri.

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-purple)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-orange)](https://github.com/vampifox)

</div>

---

## 🚀 Quick Start (Windows)

### 1. Pertama kali — jalankan setup wizard

```powershell
# Buka PowerShell sebagai Administrator
.\scripts\setup-windows.ps1
```

Script ini otomatis menginstall: **Go**, **Git**, **Docker Desktop**, **VS Code**, dan extension yang dibutuhkan.

### 2. Cek semua dependency

```powershell
.\scripts\vfx.ps1 check
```

### 3. Nyalakan stack development (PostgreSQL, Redis, MinIO, dll)

```powershell
.\scripts\vfx.ps1 docker-up
```

### 4. Bangunkan VampiFox!

```powershell
.\scripts\vfx.ps1 awaken
```

---

## 📋 Semua Perintah (`vfx.ps1`)

| Perintah | Fungsi |
|----------|--------|
| `vfx.ps1 help` | Tampilkan semua perintah |
| `vfx.ps1 awaken` | Jalankan server (dev mode) |
| `vfx.ps1 build` | Build `vampifox.exe` |
| `vfx.ps1 build-foxctl` | Build `foxctl.exe` (CLI tool) |
| `vfx.ps1 test` | Jalankan semua test |
| `vfx.ps1 test-cover` | Test + buka laporan coverage |
| `vfx.ps1 docker-up` | Nyalakan stack dev |
| `vfx.ps1 docker-down` | Matikan stack dev |
| `vfx.ps1 migrate-up` | Jalankan migrasi DB |
| `vfx.ps1 migrate-down` | Rollback migrasi |
| `vfx.ps1 migrate-create add_users` | Buat file migrasi baru |
| `vfx.ps1 check` | Cek semua dependency |

> **Tip:** Bisa juga pakai `vfx.bat` dari CMD biasa — hasilnya sama.

---

## 🏗️ Arsitektur

```
vampifox/
├── cmd/
│   ├── vampifox/        # Entry point server utama
│   └── foxctl/          # CLI tool (scaffold, migrate, dll)
│
├── internal/
│   ├── den/             # 🏠 Dependency injection & lifecycle
│   ├── fangs/           # 🦷 Database layer (PostgreSQL/GORM)
│   ├── shadow/          # 👤 Cache layer (Redis)
│   ├── core/
│   │   ├── tenant/      # Multi-tenancy engine
│   │   ├── auth/        # JWT Sanctum (autentikasi)
│   │   ├── user/        # User management
│   │   ├── rbac/        # Role-based access control
│   │   └── audit/       # Audit trail
│   ├── modules/
│   │   ├── accounting/  # Akuntansi & keuangan
│   │   ├── inventory/   # Manajemen stok
│   │   ├── purchasing/  # Pembelian
│   │   ├── sales/       # Penjualan
│   │   ├── hrm/         # SDM & penggajian
│   │   ├── crm/         # Manajemen pelanggan
│   │   ├── project/     # Manajemen proyek
│   │   └── assets/      # Aset tetap
│   └── api/
│       ├── rest/v1/     # REST API
│       ├── graphql/     # GraphQL API
│       ├── webhook/     # Webhook handler
│       └── middleware/  # Bloodgate (auth), Covenant (rbac), dll
│
├── pkg/
│   ├── foxutil/         # Utilities umum
│   ├── bloodtype/       # Type definitions bersama
│   ├── nightvision/     # Logging & observability
│   └── moonphase/       # Scheduler
│
├── deploy/
│   ├── docker/          # Dockerfile + docker-compose
│   ├── k8s/             # Kubernetes manifests
│   └── helm/            # Helm charts
│
├── migrations/          # SQL migration files
├── configs/             # YAML configuration
├── scripts/             # vfx.ps1, vfx.bat, setup-windows.ps1
└── .vscode/             # VS Code settings & launch config
```

---

## 🧛 Kamus Istilah VampiFox

| Istilah | Arti Teknis |
|---------|-------------|
| **Den** | DI Container & lifecycle manager |
| **Fangs** | Koneksi database (PostgreSQL) |
| **Shadow** | Cache layer (Redis) |
| **Sanctum** | JWT auth manager |
| **Bloodgate** | Auth middleware |
| **Covenant** | RBAC middleware |
| **Overlord** | Role: pemilik tenant |
| **Elder Vampire** | Role: admin |
| **Daywalker** | Role: manager |
| **Familiar** | Role: staff |
| **Spectre** | Role: auditor (read-only) |
| **Awaken** | Start server |
| **Slumber** | Graceful shutdown |
| **Moonphase** | Job scheduler |

---

## 🛠️ Tech Stack

| Layer | Teknologi |
|-------|-----------|
| Language | **Go 1.23+** |
| Web Framework | **Gin** |
| ORM | **GORM** + PostgreSQL |
| Cache | **Redis** |
| Queue | **NATS JetStream** |
| Storage | **MinIO** (S3-compatible) |
| Auth | **JWT** (golang-jwt) |
| Config | **Viper** |
| Logging | **Zap** |
| GraphQL | **gqlgen** |
| Scheduler | **robfig/cron** |
| Container | **Docker** + **Docker Compose** |

---

## 📄 Lisensi

MIT License — bebas digunakan, dimodifikasi, dan didistribusikan.

---

<div align="center">
Made with 🧛🦊 by VampiFox Team
</div>
