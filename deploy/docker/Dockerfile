# ─── Stage 1: Builder ──────────────────────────────────────────
# Build dilakukan di dalam container Linux, bukan di host Windows.
# Tidak ada masalah CGO, toolchain, dll.
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Download dependencies dulu (cache layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build binary (CGO off — static binary)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-s -w" \
    -o /bin/vampifox \
    ./cmd/vampifox

# ─── Stage 2: Runtime ──────────────────────────────────────────
# Pakai distroless/static (bukan 'scratch') agar lebih kompatibel
# dan tetap menyertakan ca-certificates & timezone data.
FROM gcr.io/distroless/static-debian12

# Copy binary dari builder
COPY --from=builder /bin/vampifox /vampifox
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

EXPOSE 8080

ENTRYPOINT ["/vampifox"]
