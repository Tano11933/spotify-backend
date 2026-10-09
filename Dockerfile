# ---- Build ----
FROM golang:1.26-alpine AS builder

WORKDIR /src

# go.mod/go.sum disalin lebih dulu supaya layer `go mod download` tidak ikut
# invalid setiap kali kode berubah; cache-nya cuma dibuang saat dependency
# berubah.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Tiga binary dari satu build: API, worker, dan seeder demo. -trimpath
# membuang path mesin build, dan -ldflags "-s -w" membuang tabel simbol/debug
# supaya binary lebih kecil. Migrasi SQL ikut ter-embed lewat go:embed, jadi
# tidak perlu file terpisah di runtime.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

# ---- Run ----
FROM alpine:3.22

# ca-certificates: verifikasi TLS saat mengirim SMTP.
# tzdata: log memakai waktu lokal container.
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -u 10001 app

WORKDIR /app

COPY --from=builder /out/api /out/worker /out/seed ./

# Direktori unggahan audio. Di compose di-mount sebagai volume supaya berkas
# tidak hilang saat container diganti.
RUN mkdir -p /app/uploads && chown -R app:app /app

# Jangan jalan sebagai root.
USER app

EXPOSE 9000

# Healthcheck memakai /health (liveness, tidak menyentuh dependency); readiness
# /ready dipakai orchestrator yang ingin tahu dependency sudah siap.
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:9000/health >/dev/null 2>&1 || exit 1

CMD ["./api"]
