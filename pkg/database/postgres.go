package database

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func ConnectPostgres() *gorm.DB {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_PORT"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		// TranslateError membuat GORM menerjemahkan error spesifik driver
		// Postgres menjadi error milik GORM sendiri — misalnya pelanggaran
		// unique constraint jadi gorm.ErrDuplicatedKey. Tanpa ini, repository
		// harus mencocokkan pesan error mentah pgx sebagai string, yang pecah
		// begitu versi driver atau versi Postgres berganti.
		TranslateError: true,
	})
	if err != nil {
		log.Fatal("Failed to connect to Postgres: ", err)
	}

	log.Println("✅ Connected to Postgres")
	return db
}
