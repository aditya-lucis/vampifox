# VampiFox - Windows First-Time Setup
# Jalankan SEKALI sebagai Administrator
# Usage: .\scripts\setup-windows.ps1

#Requires -RunAsAdministrator

$ErrorActionPreference = "Stop"

function Write-Step {
    param([string]$msg)
    Write-Host ""
    Write-Host "  >> $msg" -ForegroundColor Cyan
}

function Write-OK {
    param([string]$msg)
    Write-Host "  [OK]   $msg" -ForegroundColor Green
}

function Write-Skip {
    param([string]$msg)
    Write-Host "  [SKIP] $msg (sudah ada)" -ForegroundColor DarkGray
}

function Write-Warn {
    param([string]$msg)
    Write-Host "  [!!]   $msg" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "  =============================================" -ForegroundColor Cyan
Write-Host "  VampiFox - Windows Setup Wizard" -ForegroundColor Cyan
Write-Host "  =============================================" -ForegroundColor Cyan
Write-Host "  Script ini menginstall semua dependency" -ForegroundColor White
Write-Host "  yang dibutuhkan untuk development VampiFox." -ForegroundColor White
Write-Host ""

# -- 1. Cek winget --
Write-Step "Memeriksa winget..."
try {
    winget --version | Out-Null
    Write-OK "winget tersedia"
} catch {
    Write-Host "  [ERROR] winget tidak ditemukan." -ForegroundColor Red
    Write-Host "  Install App Installer dari Microsoft Store." -ForegroundColor Gray
    exit 1
}

# -- 2. Go --
Write-Step "Memeriksa Go..."
$goFound = $false
try {
    $goVer = go version 2>&1
    Write-Skip "Go sudah ada: $goVer"
    $goFound = $true
} catch {
    $goFound = $false
}
if (-not $goFound) {
    Write-Host "  Menginstall Go..." -ForegroundColor Yellow
    winget install --id GoLang.Go --silent --accept-package-agreements --accept-source-agreements
    Write-OK "Go terinstall. Restart terminal setelah setup selesai."
}

# -- 3. Git --
Write-Step "Memeriksa Git..."
$gitFound = $false
try {
    git --version | Out-Null
    Write-Skip "Git sudah ada"
    $gitFound = $true
} catch {
    $gitFound = $false
}
if (-not $gitFound) {
    Write-Host "  Menginstall Git..." -ForegroundColor Yellow
    winget install --id Git.Git --silent --accept-package-agreements --accept-source-agreements
    Write-OK "Git terinstall"
}

# -- 4. Docker Desktop --
Write-Step "Memeriksa Docker Desktop..."
$dockerFound = $false
try {
    docker --version | Out-Null
    Write-Skip "Docker sudah ada"
    $dockerFound = $true
} catch {
    $dockerFound = $false
}
if (-not $dockerFound) {
    Write-Warn "Docker Desktop butuh restart Windows setelah install."
    winget install --id Docker.DockerDesktop --silent --accept-package-agreements --accept-source-agreements
    Write-OK "Docker Desktop terinstall"
}

# -- 5. VS Code --
Write-Step "Memeriksa VS Code..."
$codeFound = $false
try {
    code --version | Out-Null
    Write-Skip "VS Code sudah ada"
    $codeFound = $true
} catch {
    $codeFound = $false
}
if (-not $codeFound) {
    winget install --id Microsoft.VisualStudioCode --silent --accept-package-agreements --accept-source-agreements
    Write-OK "VS Code terinstall"
}

# -- 6. golangci-lint --
Write-Step "Memeriksa golangci-lint..."
$lintFound = $false
try {
    golangci-lint --version | Out-Null
    Write-Skip "golangci-lint sudah ada"
    $lintFound = $true
} catch {
    $lintFound = $false
}
if (-not $lintFound) {
    try {
        winget install --id golangci-lint.golangci-lint --silent --accept-package-agreements --accept-source-agreements 2>$null
        Write-OK "golangci-lint terinstall via winget"
    } catch {
        Write-Warn "golangci-lint tidak tersedia via winget, skip. Install manual jika diperlukan."
    }
}

# -- 7. VS Code Extensions --
Write-Step "Menginstall VS Code extensions untuk Go..."
$extensions = @(
    "golang.go",
    "ms-azuretools.vscode-docker",
    "mtxr.sqltools",
    "eamodio.gitlens",
    "redhat.vscode-yaml"
)
$codeAvailable = $false
try {
    code --version | Out-Null
    $codeAvailable = $true
} catch {}

if ($codeAvailable) {
    foreach ($ext in $extensions) {
        try {
            code --install-extension $ext --force 2>$null | Out-Null
        } catch {}
    }
    Write-OK "VS Code extensions terinstall"
} else {
    Write-Warn "VS Code belum tersedia di PATH, skip extensions. Restart terminal lalu jalankan ulang."
}

# -- 8. GOPATH di PATH --
Write-Step "Memeriksa GOPATH di PATH..."
try {
    $GoPath = (go env GOPATH 2>$null)
    if ($GoPath) {
        $GoBin = Join-Path $GoPath "bin"
        $CurrentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
        if ($CurrentPath -notlike "*$GoBin*") {
            [Environment]::SetEnvironmentVariable("PATH", "$CurrentPath;$GoBin", "User")
            Write-OK "GOPATH\bin ditambahkan ke PATH (berlaku setelah restart terminal)"
        } else {
            Write-Skip "GOPATH\bin sudah di PATH"
        }
    }
} catch {
    Write-Warn "Go belum tersedia di PATH sekarang, GOPATH tidak bisa dicek. Restart terminal dulu."
}

# -- 9. Buat folder bin --
Write-Step "Mempersiapkan folder bin..."
$Root = Split-Path -Parent $PSScriptRoot
$BinDir = Join-Path $Root "bin"
if (-not (Test-Path $BinDir)) {
    New-Item -ItemType Directory -Path $BinDir | Out-Null
    Write-OK "Folder bin\ dibuat"
} else {
    Write-Skip "Folder bin\ sudah ada"
}

# -- Selesai --
Write-Host ""
Write-Host "  =============================================" -ForegroundColor Green
Write-Host "  Setup selesai!" -ForegroundColor Green
Write-Host "  =============================================" -ForegroundColor Green
Write-Host ""
Write-Host "  Langkah selanjutnya:" -ForegroundColor White
Write-Host "  1. Tutup dan buka ulang PowerShell" -ForegroundColor Cyan
Write-Host "  2. .\scripts\vfx.ps1 check" -ForegroundColor Cyan
Write-Host "  3. .\scripts\vfx.ps1 docker-up" -ForegroundColor Cyan
Write-Host "  4. .\scripts\vfx.ps1 awaken" -ForegroundColor Cyan
Write-Host ""
