// Package logging menyiapkan slog sebagai logger aplikasi.
//
// Tujuannya dua: output terstruktur (JSON di produksi, text di terminal) dan
// jembatan untuk panggilan log.Printf lama. Panggilan lama TIDAK diubah satu
// per satu; bridge di bawah meneruskannya ke slog supaya formatnya seragam
// tanpa diff besar yang berisiko.
package logging

import (
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
)

// Setup mengonfigurasi logger default aplikasi.
//
// format "json" dipakai di produksi (mudah di-parse agregator log); selain itu
// handler text yang enak dibaca di terminal development.
func Setup(format string) {
	SetupTo(os.Stdout, format)
}

// SetupTo sama seperti Setup tetapi menulis ke writer tertentu. Dipakai test
// supaya bisa memeriksa output tanpa menangkap stdout proses.
func SetupTo(w io.Writer, format string) {
	options := &slog.HandlerOptions{Level: slog.LevelInfo}

	var handler slog.Handler
	if strings.EqualFold(format, "json") {
		handler = slog.NewJSONHandler(w, options)
	} else {
		handler = slog.NewTextHandler(w, options)
	}

	slog.SetDefault(slog.New(handler))

	// Panggilan log.Printf yang tersisa ikut dialirkan ke slog. log.SetFlags(0)
	// mematikan prefix tanggal/waktu bawaan karena slog sudah menambahkan
	// timestamp terstruktur.
	log.SetFlags(0)
	log.SetOutput(&bridge{logger: slog.Default()})
}

// bridge adalah io.Writer yang meneruskan setiap baris log standar ke slog
// sebagai satu record.
type bridge struct {
	logger *slog.Logger
}

func (b *bridge) Write(p []byte) (int, error) {
	line := strings.TrimRight(string(p), "\r\n")
	if line != "" {
		b.logger.Info(line)
	}
	return len(p), nil
}
