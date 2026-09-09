// Command seed mengisi database dengan data demo yang realistis.
//
// Kenapa command Go terpisah, bukan file .sql?
//
//   - Password user WAJIB di-hash bcrypt. File SQL tidak bisa melakukannya,
//     jadi hash harus di-generate di luar lalu ditempel — dan begitu cost
//     bcrypt di auth_service berubah, hash tempelan itu jadi usang tanpa ada
//     yang menyadarinya.
//   - Seeder ini memakai model.User, model.Artist, dst. Kalau nanti ada kolom
//     baru ditambahkan ke struct, seeder GAGAL COMPILE — bukan diam-diam
//     mengisi data yang tidak lengkap seperti yang terjadi pada file SQL.
//   - Hook GORM (mis. User.BeforeCreate yang men-generate UUID) ikut jalan.
//
// Pemakaian:
//
//	go run ./cmd/seed            # isi database, menolak jalan kalau sudah ada data
//	go run ./cmd/seed -fresh     # KOSONGKAN katalog lebih dulu, lalu isi ulang
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"spotify-backend/internal/model"
	"spotify-backend/pkg/cache"
	"spotify-backend/pkg/database"
)

// bcryptCost sengaja disamakan dengan konstanta di internal/service/auth_service.go.
//
// Nilainya tidak bisa diimpor karena di sana huruf awalnya kecil (unexported).
// Menyalinnya dengan catatan ini lebih jujur daripada mengekspor konstanta
// internal hanya demi seeder — tapi kalau nilai di sana berubah, ubah juga di
// sini supaya password user demo tidak lebih lemah dari user sungguhan.
const bcryptCost = 12

func main() {
	fresh := flag.Bool("fresh", false,
		"kosongkan tabel katalog (artists, albums, songs, playlists) sebelum mengisi ulang")
	flag.Parse()

	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system env")
	}

	db := database.ConnectPostgres()
	rdb := cache.ConnectRedis()

	// AutoMigrate dijalankan di sini juga, urutannya sama persis dengan main.go.
	// Tujuannya supaya seeder bisa dipakai pada database yang benar-benar baru,
	// tanpa harus menjalankan API dulu.
	if err := db.AutoMigrate(
		&model.User{},
		&model.Artist{},
		&model.Album{},
		&model.Song{},
		&model.Playlist{},
	); err != nil {
		log.Fatal("Failed to run migrations: ", err)
	}

	start := time.Now()

	if *fresh {
		if err := truncateCatalog(db); err != nil {
			log.Fatal("Failed to truncate: ", err)
		}
		log.Println("🧹 Katalog dikosongkan (-fresh)")
	} else if err := ensureCatalogEmpty(db); err != nil {
		log.Fatal(err)
	}

	users, err := seedUsers(db)
	if err != nil {
		log.Fatal("Failed to seed users: ", err)
	}

	catalog, err := seedCatalog(db)
	if err != nil {
		log.Fatal("Failed to seed catalog: ", err)
	}

	playlistCount, err := seedPlaylists(db, users, catalog.songs)
	if err != nil {
		log.Fatal("Failed to seed playlists: ", err)
	}

	// LANGKAH YANG PALING MUDAH TERLUPAKAN.
	//
	// Seeder menulis LANGSUNG ke Postgres, melewati service layer — artinya
	// invalidasi cache yang biasanya dilakukan ArtistService/AlbumService tidak
	// pernah terjadi. Kalau Redis masih memegang "artists:all" dari sebelum
	// seeding (TTL 5 menit), frontend akan menampilkan katalog KOSONG selama
	// beberapa menit meski database sudah penuh — dan gejalanya sangat
	// membingungkan karena datanya jelas-jelas ada saat dicek lewat psql.
	flushed, err := flushCatalogCache(context.Background(), rdb)
	if err != nil {
		log.Printf("⚠️  Gagal membersihkan cache: %v (data tetap masuk, tunggu TTL habis atau restart Redis)", err)
	} else {
		log.Printf("🧽 Cache katalog dibersihkan (%d key dihapus)", flushed)
	}

	printSummary(users, catalog, playlistCount, time.Since(start))
}

/* ==========================================================================
 * Guard & pembersihan
 * ======================================================================== */

// ensureCatalogEmpty menolak seeding kalau sudah ada data.
//
// Tanpa penjaga ini, menjalankan seeder dua kali menghasilkan katalog ganda —
// dan karena artist tidak punya unique constraint pada nama, database menerimanya
// tanpa keluhan sama sekali.
func ensureCatalogEmpty(db *gorm.DB) error {
	var count int64
	if err := db.Model(&model.Artist{}).Count(&count).Error; err != nil {
		return fmt.Errorf("gagal menghitung artist: %w", err)
	}

	if count > 0 {
		return fmt.Errorf(
			"database sudah berisi %d artist — seeding dibatalkan.\n"+
				"Jalankan ulang dengan -fresh kalau memang ingin MENGHAPUS katalog lama:\n"+
				"    go run ./cmd/seed -fresh", count)
	}
	return nil
}

// truncateCatalog mengosongkan tabel katalog dan playlist.
//
// Tabel users SENGAJA TIDAK ikut dikosongkan. Akun yang kamu daftarkan sendiri
// lewat halaman register (termasuk password yang kamu hafal) tidak akan hilang
// hanya karena kamu ingin mengganti daftar lagu.
//
// RESTART IDENTITY mengembalikan sequence ID ke 1, jadi URL demo tetap rapi
// (/albums/1). CASCADE diperlukan karena songs punya foreign key ke albums.
func truncateCatalog(db *gorm.DB) error {
	return db.Exec(
		"TRUNCATE TABLE playlist_songs, playlists, songs, albums, artists RESTART IDENTITY CASCADE",
	).Error
}

/* ==========================================================================
 * Users
 * ======================================================================== */

func seedUsers(db *gorm.DB) ([]model.User, error) {
	users := make([]model.User, 0, len(userSeeds))

	for _, seed := range userSeeds {
		// Cari dulu berdasarkan email. Kalau akunnya sudah ada (mis. sisa
		// seeding sebelumnya, karena truncateCatalog tidak menyentuh users),
		// pakai yang lama alih-alih gagal karena unique constraint.
		//
		// Find + Limit(1) dipakai, BUKAN First(). Keduanya sama-sama mengambil
		// satu baris, tapi First() mengembalikan gorm.ErrRecordNotFound saat
		// kosong — dan logger GORM mencetak itu sebagai ERROR merah di terminal.
		// Padahal "user demo ini memang belum ada" adalah keadaan yang normal,
		// bahkan yang paling sering terjadi. Find() cukup melaporkannya lewat
		// RowsAffected, tanpa error dan tanpa log yang menyesatkan.
		var existing []model.User
		if err := db.Where("email = ?", seed.Email).Limit(1).Find(&existing).Error; err != nil {
			return nil, fmt.Errorf("gagal mencari user %s: %w", seed.Email, err)
		}

		if len(existing) > 0 {
			users = append(users, existing[0])
			continue
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(seed.Password), bcryptCost)
		if err != nil {
			return nil, fmt.Errorf("gagal hash password %s: %w", seed.Email, err)
		}

		role := model.RoleUser
		if seed.IsAdmin {
			role = model.RoleAdmin
		}

		user := model.User{
			Name:  seed.Name,
			Email: seed.Email,
			// Field-nya PasswordHash, bukan Password — nilai yang disimpan
			// adalah hash, tidak pernah teks aslinya.
			PasswordHash: string(hash),
			Role:         role,
		}

		if err := db.Create(&user).Error; err != nil {
			return nil, fmt.Errorf("gagal membuat user %s: %w", seed.Email, err)
		}

		users = append(users, user)
	}

	return users, nil
}

/* ==========================================================================
 * Katalog
 * ======================================================================== */

type seededCatalog struct {
	artists []model.Artist
	albums  []model.Album
	songs   []model.Song
}

func seedCatalog(db *gorm.DB) (seededCatalog, error) {
	var result seededCatalog

	// PCG dengan seed tetap, bukan waktu sekarang.
	//
	// Artinya durasi lagu dan tanggal rilis SELALU SAMA setiap kali seeder
	// dijalankan. Ini penting untuk demo dan screenshot: hasilnya bisa
	// direproduksi, dan kalau ada yang aneh, orang lain bisa mendapatkan
	// database yang persis sama untuk diperiksa.
	rng := rand.New(rand.NewPCG(20260805, 9000))

	/* --- Artist ---------------------------------------------------------- */
	artists := make([]model.Artist, len(artistSeeds))
	for i, seed := range artistSeeds {
		artists[i] = model.Artist{
			Name:     seed.Name,
			Bio:      seed.Bio,
			ImageURL: pictureURL(seed.Name, 400),
		}
	}

	// CreateInBatches menggabungkan banyak baris menjadi satu perintah INSERT
	// per batch. Untuk ratusan lagu, ini selisih antara ratusan round-trip ke
	// Postgres dan segelintir saja. ID hasil generate ditulis balik ke slice.
	if err := db.CreateInBatches(&artists, 50).Error; err != nil {
		return result, fmt.Errorf("insert artists: %w", err)
	}

	/* --- Album ----------------------------------------------------------- */
	// albumOwner menyimpan asal-usul tiap album (artist ke berapa, album ke
	// berapa dalam daftarnya) supaya setelah INSERT kita bisa memasangkan ID
	// yang baru dibuat dengan daftar lagunya.
	type albumOrigin struct{ artistIdx, albumIdx int }

	var albums []model.Album
	var origins []albumOrigin

	for artistIdx, seed := range artistSeeds {
		for albumIdx, albumSeed := range seed.Albums {
			albums = append(albums, model.Album{
				Title:    albumSeed.Title,
				CoverURL: pictureURL(albumSeed.Title, 500),
				ReleaseDate: time.Date(
					albumSeed.Year, albumSeed.Month, 1+rng.IntN(28),
					0, 0, 0, 0, time.UTC,
				),
				ArtistID: artists[artistIdx].ID,
			})
			origins = append(origins, albumOrigin{artistIdx, albumIdx})
		}
	}

	if err := db.CreateInBatches(&albums, 50).Error; err != nil {
		return result, fmt.Errorf("insert albums: %w", err)
	}

	/* --- Song ------------------------------------------------------------ */
	var songs []model.Song

	for i, origin := range origins {
		albumSeed := artistSeeds[origin.artistIdx].Albums[origin.albumIdx]

		// Variabel baru per iterasi, supaya pointer di bawah menunjuk ke nilai
		// album ini — bukan ke satu variabel bersama yang terus tertimpa.
		albumID := albums[i].ID

		for _, title := range albumSeed.Songs {
			songs = append(songs, model.Song{
				Title:    title,
				Duration: randomDuration(rng),
				FileURL:  audioURL(title),
				ArtistID: artists[origin.artistIdx].ID,
				AlbumID:  &albumID,
			})
		}
	}

	// Single: lagu tanpa album, AlbumID sengaja nil supaya kolomnya NULL.
	for artistIdx, seed := range artistSeeds {
		for _, title := range seed.Singles {
			songs = append(songs, model.Song{
				Title:    title,
				Duration: randomDuration(rng),
				FileURL:  audioURL(title),
				ArtistID: artists[artistIdx].ID,
				AlbumID:  nil,
			})
		}
	}

	if err := db.CreateInBatches(&songs, 200).Error; err != nil {
		return result, fmt.Errorf("insert songs: %w", err)
	}

	result.artists = artists
	result.albums = albums
	result.songs = songs
	return result, nil
}

/* ==========================================================================
 * Playlist
 * ======================================================================== */

func seedPlaylists(db *gorm.DB, users []model.User, songs []model.Song) (int, error) {
	if len(users) == 0 || len(songs) == 0 {
		return 0, nil
	}

	created := 0

	for _, seed := range playlistSeeds {
		if seed.OwnerIndex >= len(users) {
			continue
		}

		// Ambil lagu berjarak SongStride supaya isinya menyebar ke banyak
		// artist. Map dipakai untuk membuang duplikat, yang bisa muncul kalau
		// stride × count melewati panjang daftar lagu.
		seen := make(map[uint]bool)
		var chosen []model.Song

		for i := 0; i < seed.SongCount; i++ {
			song := songs[(seed.SongOffset+i*seed.SongStride)%len(songs)]
			if seen[song.ID] {
				continue
			}
			seen[song.ID] = true
			chosen = append(chosen, song)
		}

		playlist := model.Playlist{
			Name:        seed.Name,
			Description: seed.Description,
			IsPublic:    seed.IsPublic,
			UserID:      users[seed.OwnerIndex].ID,
			Songs:       chosen,
		}

		// Omit("Songs.*") KRUSIAL di sini.
		//
		// Secara default, GORM menganggap struct Song di dalam Playlist sebagai
		// data yang juga perlu disimpan, sehingga ia menjalankan upsert ke tabel
		// songs. Lagu-lagu itu sudah ada dan tidak berubah — yang kita inginkan
		// hanya baris di tabel perantara playlist_songs. Tanpa Omit, seeder
		// menulis ulang ratusan baris lagu tanpa alasan.
		if err := db.Omit("Songs.*").Create(&playlist).Error; err != nil {
			return created, fmt.Errorf("insert playlist %q: %w", seed.Name, err)
		}

		created++
	}

	return created, nil
}

/* ==========================================================================
 * Cache
 * ======================================================================== */

// flushCatalogCache menghapus key cache katalog, dan HANYA key katalog.
//
// FLUSHDB akan jauh lebih singkat, tapi juga akan menghapus refresh_token:*
// (semua orang ter-logout paksa) dan penghitung rate limit. Seeder tidak punya
// urusan dengan keduanya.
//
// Pola key-nya diambil dari internal/service/cache.go: "artists:all",
// "albums:all", "artist:<id>", "album:<id>".
func flushCatalogCache(ctx context.Context, rdb *redis.Client) (int, error) {
	patterns := []string{"artists:all", "albums:all", "artist:*", "album:*"}
	deleted := 0

	for _, pattern := range patterns {
		var cursor uint64

		// SCAN dipakai, bukan KEYS. KEYS memblokir seluruh Redis sampai selesai
		// memindai — tidak masalah pada database demo, tapi merupakan kebiasaan
		// buruk yang berakibat fatal di produksi. SCAN memindai sedikit demi
		// sedikit tanpa mengunci apa pun.
		for {
			keys, next, err := rdb.Scan(ctx, cursor, pattern, 100).Result()
			if err != nil {
				return deleted, err
			}

			if len(keys) > 0 {
				n, err := rdb.Del(ctx, keys...).Result()
				if err != nil {
					return deleted, err
				}
				deleted += int(n)
			}

			cursor = next
			if cursor == 0 {
				break
			}
		}
	}

	return deleted, nil
}

/* ==========================================================================
 * Helper
 * ======================================================================== */

// randomDuration menghasilkan durasi 2:30 sampai 5:29 dalam detik.
func randomDuration(rng *rand.Rand) int {
	return 150 + rng.IntN(180)
}

// pictureURL memakai picsum.photos — layanan gambar placeholder publik.
//
// Parameter /seed/<slug>/ membuat gambar untuk judul yang sama SELALU sama,
// bukan acak tiap kali dimuat. Tanpa itu, cover album berubah setiap refresh
// dan tampilannya terasa rusak.
//
// Ganti fungsi ini kalau nanti kamu punya storage sendiri.
func pictureURL(name string, size int) string {
	return fmt.Sprintf("https://picsum.photos/seed/%s/%d/%d", slugify(name), size, size)
}

// audioURL memakai satu file demo publik CC0 dari MDN.
//
// Ini sengaja satu file untuk seluruh katalog demo: seeder tidak perlu
// mendistribusikan puluhan file audio hanya untuk menguji alur player.
func audioURL(title string) string {
	return "https://interactive-examples.mdn.mozilla.net/media/cc0-audio/t-rex-roar.mp3"
}

// slugify mengubah "Kereta Terakhir" menjadi "kereta-terakhir".
func slugify(value string) string {
	var builder strings.Builder
	pendingDash := false

	for _, r := range strings.ToLower(value) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			if pendingDash && builder.Len() > 0 {
				builder.WriteByte('-')
			}
			pendingDash = false
			builder.WriteRune(r)
		default:
			// Tandai perlunya pemisah, tapi jangan tulis sekarang — supaya
			// spasi beruntun atau tanda baca di akhir tidak menghasilkan
			// "--" atau tanda hubung menggantung.
			pendingDash = true
		}
	}

	return builder.String()
}

func printSummary(users []model.User, catalog seededCatalog, playlists int, elapsed time.Duration) {
	singles := 0
	for _, song := range catalog.songs {
		if song.AlbumID == nil {
			singles++
		}
	}

	log.Println("")
	log.Println(" Seeding selesai dalam", elapsed.Round(time.Millisecond))
	log.Printf("   %3d user", len(users))
	log.Printf("   %3d artist", len(catalog.artists))
	log.Printf("   %3d album", len(catalog.albums))
	log.Printf("   %3d lagu (%d di antaranya single tanpa album)", len(catalog.songs), singles)
	log.Printf("   %3d playlist", playlists)
	log.Println("")
	log.Println("   Akun demo:")
	for _, seed := range userSeeds {
		role := "user"
		if seed.IsAdmin {
			role = "admin"
		}
		log.Printf("     %-22s %-14s (%s)", seed.Email, seed.Password, role)
	}
	log.Println("")
}
