package app

import (
	"log"
	"os"
	"time"
)

// EnvDuration membaca durasi bergaya Go ("15m", "168h", "5m30s") dari
// environment, dengan nilai default kalau kosong atau tidak valid.
//
// Dipakai API dan worker; nilai yang tidak valid hanya di-log dan memakai
// default supaya salah ketik tidak mematikan aplikasi.
func EnvDuration(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}

	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Printf("Invalid duration %s=%q, using default %s", key, raw, fallback)
		return fallback
	}
	return d
}
