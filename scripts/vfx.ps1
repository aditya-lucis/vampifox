param(
    [Parameter(Position=0)]
    [string]$Command = "help",

    [Parameter(Position=1)]
    [string]$Arg1 = ""
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$Binary = "vampifox"
$BinDir = Join-Path $Root "bin"

try {
    $Version = git describe --tags --always 2>$null
    if (-not $Version) { $Version = "0.1.0-nightfall" }
} catch {
    $Version = "0.1.0-nightfall"
}

function Write-Header {
    Write-Host ""
    Write-Host "  [VampiFox] Application Framework v$Version" -ForegroundColor Cyan
    Write-Host "  -----------------------------------------" -ForegroundColor DarkGray
    Write-Host ""
}

function Show-Help {
    Write-Header
    Write-Host "  PERINTAH TERSEDIA:" -ForegroundColor White
    Write-Host ""
    $cmds = @(
        @("awaken",         "Jalankan VampiFox (development mode)"),
        @("build",          "Build binary vampifox.exe"),
        @("build-foxctl",   "Build CLI tool foxctl.exe"),
        @("test",           "Jalankan semua unit test"),
        @("test-cover",     "Test + buka laporan coverage di browser"),
        @("lint",           "Linting kode (butuh golangci-lint)"),
        @("tidy",           "go mod tidy"),
        @("docker-up",      "Nyalakan stack dev (Postgres, Redis, dll)"),
        @("docker-down",    "Matikan stack dev"),
        @("docker-build",   "Build Docker image VampiFox"),
        @("migrate-up",     "Jalankan migrasi database"),
        @("migrate-down",   "Rollback migrasi terakhir"),
        @("migrate-create", "Buat migrasi baru: vfx.ps1 migrate-create add_users"),
        @("check",          "Cek semua dependency (Go, Docker, Git)")
    )
    foreach ($c in $cmds) {
        Write-Host ("  {0,-18}" -f $c[0]) -NoNewline -ForegroundColor Yellow
        Write-Host $c[1]
    }
    Write-Host ""
}

function Invoke-Awaken {
    Write-Host "[VampiFox] Sedang terbangun..." -ForegroundColor Magenta
    Set-Location $Root
    go run ./cmd/vampifox
}

function Invoke-Build {
    Write-Host "[VampiFox] Membangun binary..." -ForegroundColor Cyan
    if (-not (Test-Path $BinDir)) {
        New-Item -ItemType Directory -Path $BinDir | Out-Null
    }
    Set-Location $Root
    $env:CGO_ENABLED = "0"
    $env:GOOS        = "windows"
    $env:GOARCH      = "amd64"
    $OutFile = Join-Path $BinDir "$Binary.exe"
    go build -ldflags="-s -w -X main.Version=$Version" -o $OutFile ./cmd/vampifox
    if ($LASTEXITCODE -eq 0) {
        Write-Host "[OK] Build sukses: $OutFile" -ForegroundColor Green
    } else {
        Write-Host "[GAGAL] Build error!" -ForegroundColor Red
        exit 1
    }
}

function Invoke-BuildFoxctl {
    Write-Host "[VampiFox] Build foxctl CLI..." -ForegroundColor Cyan
    if (-not (Test-Path $BinDir)) {
        New-Item -ItemType Directory -Path $BinDir | Out-Null
    }
    Set-Location $Root
    $env:CGO_ENABLED = "0"
    $env:GOOS        = "windows"
    $env:GOARCH      = "amd64"
    $OutFile = Join-Path $BinDir "foxctl.exe"
    go build -ldflags="-s -w -X main.Version=$Version" -o $OutFile ./cmd/foxctl
    if ($LASTEXITCODE -eq 0) {
        Write-Host "[OK] foxctl siap: $OutFile" -ForegroundColor Green
    } else {
        Write-Host "[GAGAL] Build foxctl error!" -ForegroundColor Red
        exit 1
    }
}

function Invoke-Test {
    Write-Host "[VampiFox] Menjalankan test..." -ForegroundColor Yellow
    Set-Location $Root
    go test ./... -v -race -coverprofile=coverage.out
}

function Invoke-TestCover {
    Invoke-Test
    Write-Host "[VampiFox] Membuka laporan coverage..." -ForegroundColor Cyan
    go tool cover -html=coverage.out
}

function Invoke-Lint {
    Write-Host "[VampiFox] Memeriksa kode..." -ForegroundColor Cyan
    golangci-lint run ./...
}

function Invoke-Tidy {
    Write-Host "[VampiFox] Membersihkan dependencies..." -ForegroundColor Cyan
    Set-Location $Root
    go mod tidy
    Write-Host "[OK] Selesai." -ForegroundColor Green
}

function Invoke-DockerUp {
    Write-Host "[VampiFox] Menyalakan stack development..." -ForegroundColor Magenta
    $ComposeFile = Join-Path $Root "deploy\docker\docker-compose.yml"
    docker compose -f $ComposeFile up -d
    if ($LASTEXITCODE -eq 0) {
        Write-Host ""
        Write-Host "[OK] Stack berjalan!" -ForegroundColor Green
        Write-Host "  PostgreSQL : localhost:5432" -ForegroundColor Cyan
        Write-Host "  Redis      : localhost:6379" -ForegroundColor Cyan
        Write-Host "  MinIO      : http://localhost:9001" -ForegroundColor Cyan
        Write-Host "  Mailpit    : http://localhost:8025" -ForegroundColor Cyan
        Write-Host "  NATS       : localhost:4222" -ForegroundColor Cyan
    } else {
        Write-Host "[GAGAL] Docker compose error!" -ForegroundColor Red
        exit 1
    }
}

function Invoke-DockerDown {
    Write-Host "[VampiFox] Mematikan stack..." -ForegroundColor Yellow
    $ComposeFile = Join-Path $Root "deploy\docker\docker-compose.yml"
    docker compose -f $ComposeFile down
    Write-Host "[OK] Stack dimatikan." -ForegroundColor Green
}

function Invoke-DockerBuild {
    Write-Host "[VampiFox] Build Docker image..." -ForegroundColor Cyan
    $DockerFile = Join-Path $Root "deploy\docker\Dockerfile"
    docker build -f $DockerFile -t "vampifox:$Version" $Root
}

function Invoke-MigrateUp {
    Write-Host "[VampiFox] Menjalankan migrasi..." -ForegroundColor Cyan
    Set-Location $Root
    go run ./cmd/foxctl migrate up
}

function Invoke-MigrateDown {
    Write-Host "[VampiFox] Rollback migrasi terakhir..." -ForegroundColor Yellow
    Set-Location $Root
    go run ./cmd/foxctl migrate down
}

function Invoke-MigrateCreate {
    if (-not $Arg1) {
        Write-Host "[ERROR] Nama migrasi wajib diisi." -ForegroundColor Red
        Write-Host "  Contoh: .\scripts\vfx.ps1 migrate-create add_users_table" -ForegroundColor Gray
        exit 1
    }
    Write-Host "[VampiFox] Membuat migrasi: $Arg1" -ForegroundColor Cyan
    Set-Location $Root
    go run ./cmd/foxctl migrate create $Arg1
}

function Invoke-Check {
    Write-Header
    Write-Host "  PEMERIKSAAN DEPENDENCY:" -ForegroundColor White
    Write-Host ""

    # Go
    try {
        $GoVer = (go version 2>&1)
        Write-Host ("  [OK]  Go        : " + $GoVer) -ForegroundColor Green
    } catch {
        Write-Host "  [!!]  Go        : TIDAK DITEMUKAN - install dari https://go.dev/dl/" -ForegroundColor Red
    }

    # Git
    try {
        $GitVer = (git --version 2>&1)
        Write-Host ("  [OK]  Git       : " + $GitVer) -ForegroundColor Green
    } catch {
        Write-Host "  [!!]  Git       : TIDAK DITEMUKAN - install dari https://git-scm.com/" -ForegroundColor Red
    }

    # Docker
    try {
        $DockerVer = (docker --version 2>&1)
        Write-Host ("  [OK]  Docker    : " + $DockerVer) -ForegroundColor Green
    } catch {
        Write-Host "  [!!]  Docker    : TIDAK DITEMUKAN - install Docker Desktop" -ForegroundColor Red
    }

    # golangci-lint (opsional)
    try {
        $LintVer = (golangci-lint version 2>&1)
        Write-Host ("  [OK]  golangci  : " + $LintVer) -ForegroundColor Green
    } catch {
        Write-Host "  [--]  golangci  : Tidak ada (opsional)" -ForegroundColor Yellow
    }

    Write-Host ""
    Write-Host "  Langkah selanjutnya:" -ForegroundColor White
    Write-Host "  1. .\scripts\vfx.ps1 docker-up" -ForegroundColor Cyan
    Write-Host "  2. .\scripts\vfx.ps1 awaken" -ForegroundColor Cyan
    Write-Host ""
}

# -- Router --
switch ($Command.ToLower()) {
    "help"           { Show-Help }
    "awaken"         { Invoke-Awaken }
    "build"          { Invoke-Build }
    "build-foxctl"   { Invoke-BuildFoxctl }
    "test"           { Invoke-Test }
    "test-cover"     { Invoke-TestCover }
    "lint"           { Invoke-Lint }
    "tidy"           { Invoke-Tidy }
    "docker-up"      { Invoke-DockerUp }
    "docker-down"    { Invoke-DockerDown }
    "docker-build"   { Invoke-DockerBuild }
    "migrate-up"     { Invoke-MigrateUp }
    "migrate-down"   { Invoke-MigrateDown }
    "migrate-create" { Invoke-MigrateCreate }
    "check"          { Invoke-Check }
    default {
        Write-Host "[ERROR] Perintah '$Command' tidak dikenal." -ForegroundColor Red
        Write-Host "  Jalankan: .\scripts\vfx.ps1 help" -ForegroundColor Gray
        exit 1
    }
}