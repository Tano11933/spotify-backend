// Package migrations menjalankan migrasi SQL yang tidak bisa diungkapkan lewat
// AutoMigrate GORM — extension Postgres, index ekspresi (tsvector), dan
// index trigram.
//
// File SQL di folder sql/ diurutkan berdasarkan nama; versi yang sudah
// dijalankan dicatat di tabel schema_migrations, jadi aman dipanggil setiap
// startup. AutoMigrate tetap menjadi cara utama membuat/mengubah tabel.
package migrations

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

//go:embed sql/*.sql
var sqlFiles embed.FS

// AppliedMigration mencatat satu versi migrasi yang sudah dieksekusi.
type AppliedMigration struct {
	Version   string    `gorm:"primaryKey;size:255"`
	AppliedAt time.Time `gorm:"not null"`
}

func (AppliedMigration) TableName() string { return "schema_migrations" }

// Run menjalankan semua migrasi yang belum pernah dieksekusi.
func Run(db *gorm.DB) error {
	if err := db.AutoMigrate(&AppliedMigration{}); err != nil {
		return fmt.Errorf("siapkan tabel schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(sqlFiles, "sql")
	if err != nil {
		return fmt.Errorf("baca folder migrasi: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		applied, err := isApplied(db, name)
		if err != nil {
			return fmt.Errorf("cek status migrasi %s: %w", name, err)
		}
		if applied {
			continue
		}

		raw, err := sqlFiles.ReadFile("sql/" + name)
		if err != nil {
			return fmt.Errorf("baca migrasi %s: %w", name, err)
		}

		if err := apply(db, name, string(raw)); err != nil {
			return fmt.Errorf("migrasi %s: %w", name, err)
		}

		log.Printf("✅ migration applied: %s", name)
	}

	return nil
}

func isApplied(db *gorm.DB, version string) (bool, error) {
	var count int64
	err := db.Model(&AppliedMigration{}).Where("version = ?", version).Count(&count).Error
	return count > 0, err
}

// apply menjalankan seluruh statement dalam satu file secara transaksional —
// kalau satu statement gagal, tidak ada yang setengah terpasang.
func apply(db *gorm.DB, version, raw string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, statement := range splitStatements(raw) {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}

		return tx.Create(&AppliedMigration{
			Version:   version,
			AppliedAt: time.Now().UTC(),
		}).Error
	})
}

// splitStatements memecah file SQL menjadi statement per ";" karena driver
// Postgres memakai prepared statement yang tidak menerima banyak perintah
// sekaligus.
//
// Pemisahan ini naif dan sengaja: file migrasi proyek ini sederhana (tanpa
// function body / dollar-quoting). Kalau nanti ada migrasi yang memuat ";" di
// dalam string, pisahkan statement-nya ke file berbeda.
func splitStatements(raw string) []string {
	parts := strings.Split(raw, ";")
	statements := make([]string, 0, len(parts))

	for _, part := range parts {
		lines := strings.Split(part, "\n")
		kept := make([]string, 0, len(lines))

		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "--") {
				continue
			}
			kept = append(kept, line)
		}

		statement := strings.TrimSpace(strings.Join(kept, "\n"))
		if statement != "" {
			statements = append(statements, statement)
		}
	}

	return statements
}
