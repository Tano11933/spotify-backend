package router

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"spotify-backend/internal/handler"
	"spotify-backend/internal/middleware"
)

type Handlers struct {
	Artist   *handler.ArtistHandler
	Song     *handler.SongHandler
	Album    *handler.AlbumHandler
	Auth     *handler.AuthHandler
	Playlist *handler.PlaylistHandler
	Search   *handler.SearchHandler
	Library  *handler.LibraryHandler
	WS       *handler.WSHandler
}

type Middlewares struct {
	Auth      *middleware.AuthMiddleware
	RateLimit *middleware.RateLimiter
}

func SetupRoutes(app *fiber.App, h *Handlers, mw *Middlewares) {
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "message": "Spotify backend is running"})
	})

	api := app.Group("/api")

	registerAuthRoutes(api, h, mw)
	registerCatalogRoutes(api, h, mw)
	registerPlaylistRoutes(api, h, mw)
	registerLibraryRoutes(api, h, mw)
	registerSearchRoutes(api, h)
	registerWebSocketRoute(app, h, mw)
}

func registerAuthRoutes(api fiber.Router, h *Handlers, mw *Middlewares) {
	auth := api.Group("/auth")

	// Setiap endpoint auth punya nama rate limiter sendiri, jadi kuotanya
	// terpisah — menghabiskan percobaan login tidak ikut memblokir register.
	//
	// Angkanya dipilih agar longgar untuk pemakaian wajar tapi cukup ketat untuk
	// menghambat serangan otomatis.

	auth.Post("/register",
		mw.RateLimit.Limit(middleware.RateLimitConfig{
			Name: "register", Max: 5, Window: time.Hour,
		}),
		h.Auth.Register,
	)

	auth.Post("/login",
		mw.RateLimit.Limit(middleware.RateLimitConfig{
			Name: "login", Max: 10, Window: 5 * time.Minute,
		}),
		h.Auth.Login,
	)

	auth.Post("/refresh",
		mw.RateLimit.Limit(middleware.RateLimitConfig{
			Name: "refresh", Max: 30, Window: time.Hour,
		}),
		h.Auth.Refresh,
	)

	// forgot-password dibatasi DUA KALI, dan itu memang perlu:
	//
	//   per email → mencegah satu korban dibombardir email reset dari banyak IP
	//   per IP    → mencegah satu penyerang menyebar spam ke ratusan email berbeda
	//
	// Batas per-IP saja tidak menutup kasus pertama, batas per-email saja tidak
	// menutup kasus kedua.
	auth.Post("/forgot-password",
		mw.RateLimit.Limit(middleware.RateLimitConfig{
			Name:    "forgot_password_email",
			Max:     3,
			Window:  time.Hour,
			KeyFunc: middleware.KeyByJSONField("email"),
		}),
		mw.RateLimit.Limit(middleware.RateLimitConfig{
			Name: "forgot_password_ip", Max: 10, Window: time.Hour,
		}),
		h.Auth.ForgotPassword,
	)

	auth.Post("/reset-password",
		mw.RateLimit.Limit(middleware.RateLimitConfig{
			Name: "reset_password", Max: 10, Window: time.Hour,
		}),
		h.Auth.ResetPassword,
	)

	// Butuh access token yang valid.
	auth.Post("/logout", mw.Auth.Protected(), h.Auth.Logout)
	auth.Get("/me", mw.Auth.Protected(), h.Auth.Me)
}

func registerCatalogRoutes(api fiber.Router, h *Handlers, mw *Middlewares) {
	// Kebijakan akses katalog:
	//
	//   READ  → publik. Halaman depan frontend harus bisa menampilkan artist dan
	//           album tanpa login, dan endpoint inilah yang di-cache di Redis.
	//   WRITE → admin saja. Menambah artist/album/lagu ke katalog adalah
	//           pekerjaan kurasi, bukan sesuatu yang boleh dilakukan user biasa
	//           yang baru mendaftar.
	//
	// Urutan middleware penting: Protected() harus lebih dulu dari
	// RequireAdmin(), karena RequireAdmin membaca role yang ditulis Protected
	// ke c.Locals.
	adminOnly := []fiber.Handler{mw.Auth.Protected(), mw.Auth.RequireAdmin()}

	artists := api.Group("/artists")
	artists.Get("/", h.Artist.GetAll)
	artists.Get("/:id", h.Artist.GetByID)
	artists.Get("/:artistId/songs", h.Song.GetByArtist)
	// Tanda ... di akhir adalah "spread" — memecah slice menjadi argumen
	// terpisah, karena Post() menerima handler secara variadic.
	artists.Post("/", chain(adminOnly, h.Artist.Create)...)
	artists.Put("/:id", chain(adminOnly, h.Artist.Update)...)
	artists.Delete("/:id", chain(adminOnly, h.Artist.Delete)...)

	albums := api.Group("/albums")
	albums.Get("/", h.Album.GetAll)
	albums.Get("/:id", h.Album.GetByID)
	albums.Post("/", chain(adminOnly, h.Album.Create)...)
	albums.Put("/:id", chain(adminOnly, h.Album.Update)...)
	albums.Delete("/:id", chain(adminOnly, h.Album.Delete)...)

	songs := api.Group("/songs")
	songs.Get("/", h.Song.GetAll)
	songs.Get("/:id", h.Song.GetByID)
	songs.Post("/", chain(adminOnly, h.Song.Create)...)
	songs.Put("/:id", chain(adminOnly, h.Song.Update)...)
	songs.Delete("/:id", chain(adminOnly, h.Song.Delete)...)
}

// chain menggabungkan daftar middleware dengan handler akhirnya menjadi satu
// slice baru.
//
// Kenapa tidak langsung `append(adminOnly, h.Artist.Create)`? Karena append
// boleh MENGGUNAKAN ULANG array di belakang slice asal kapasitasnya masih cukup.
// Kalau itu terjadi, pemanggilan chain kedua akan menimpa handler yang ditulis
// pemanggilan pertama — dan hasilnya route yang tertukar handler-nya, bug yang
// sangat sulit ditebak dari gejalanya.
//
// Dalam kasus ini slice adminOnly kebetulan len == cap sehingga append selalu
// mengalokasi array baru dan aman. Tapi bergantung pada "kebetulan" seperti itu
// bukan kebiasaan yang baik: menambah satu middleware saja bisa mengubah
// kapasitasnya dan menghidupkan bug tersebut. make() dengan kapasitas eksplisit
// menghilangkan ketergantungan itu.
func chain(middlewares []fiber.Handler, final fiber.Handler) []fiber.Handler {
	result := make([]fiber.Handler, 0, len(middlewares)+1)
	result = append(result, middlewares...)
	return append(result, final)
}

func registerPlaylistRoutes(api fiber.Router, h *Handlers, mw *Middlewares) {
	// Berbeda dari katalog, playlist adalah data MILIK user, bukan data publik
	// yang dikurasi admin. Jadi kebijakannya juga berbeda: tidak ada endpoint
	// admin-only di sini, tapi SEMUA endpoint butuh login — termasuk yang
	// membaca, karena tanpa identitas server tidak bisa tahu playlist siapa yang
	// harus ditampilkan.
	//
	// Otorisasi per-baris (siapa boleh mengubah playlist mana) tidak bisa
	// diselesaikan middleware, karena jawabannya bergantung pada isi database.
	// Itu ditangani PlaylistService lewat mustOwn().
	playlists := api.Group("/playlists", mw.Auth.Protected())

	// /public didaftarkan SEBELUM /:id. Urutan ini wajib: Fiber mencocokkan
	// route dari atas ke bawah, jadi kalau /:id lebih dulu, ia akan menangkap
	// "public" sebagai nilai parameter id dan /playlists/public tidak akan
	// pernah tercapai.
	playlists.Get("/public", h.Playlist.GetPublic)

	playlists.Post("/", h.Playlist.Create)
	playlists.Get("/", h.Playlist.GetMine)
	playlists.Get("/:id", h.Playlist.GetByID)
	playlists.Put("/:id", h.Playlist.Update)
	playlists.Delete("/:id", h.Playlist.Delete)

	playlists.Post("/:id/songs", h.Playlist.AddSong)
	playlists.Delete("/:id/songs/:songId", h.Playlist.RemoveSong)
}

// registerLibraryRoutes mendaftarkan pustaka pribadi user: liked songs,
// album tersimpan, dan artist yang diikuti.
//
// Semua endpoint butuh login dan selalu bekerja pada user yang sedang login —
// tidak ada parameter user di path, jadi tidak ada permukaan untuk IDOR.
func registerLibraryRoutes(api fiber.Router, h *Handlers, mw *Middlewares) {
	me := api.Group("/me", mw.Auth.Protected())

	me.Put("/tracks/:songId", h.Library.SaveTrack)
	me.Delete("/tracks/:songId", h.Library.RemoveTrack)
	me.Get("/tracks", h.Library.GetTracks)
	me.Get("/tracks/contains", h.Library.TracksContain)

	me.Put("/albums/:albumId", h.Library.SaveAlbum)
	me.Delete("/albums/:albumId", h.Library.RemoveAlbum)
	me.Get("/albums", h.Library.GetAlbums)
	me.Get("/albums/contains", h.Library.AlbumsContain)

	me.Put("/following/:artistId", h.Library.FollowArtist)
	me.Delete("/following/:artistId", h.Library.UnfollowArtist)
	me.Get("/following", h.Library.GetFollowing)
	me.Get("/following/contains", h.Library.FollowingContain)
}

// registerSearchRoutes mendaftarkan pencarian katalog.
//
// Search tidak di-cache: hasilnya bergantung pada query dan index GIN sudah
// membuat pencariannya murah. Endpoint ini publik — halaman pencarian harus
// bisa dipakai tanpa login.
func registerSearchRoutes(api fiber.Router, h *Handlers) {
	api.Get("/search", h.Search.Search)
}

func registerWebSocketRoute(app *fiber.App, h *Handlers, mw *Middlewares) {	// /ws didaftarkan di app, bukan di grup /api, karena WebSocket bukan
	// endpoint REST.
	//
	// Rantainya: autentikasi → pastikan ini benar-benar upgrade → tangani socket.
	//
	// ProtectedAllowQueryToken (bukan Protected) dipakai di sini karena browser
	// tidak bisa mengirim header Authorization saat membuka WebSocket — token
	// harus lewat ?token=...
	app.Get("/ws",
		mw.Auth.ProtectedAllowQueryToken(),
		h.WS.UpgradeGuard,
		h.WS.Handle(),
	)
}
