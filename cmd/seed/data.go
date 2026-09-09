package main

import "time"

// File ini HANYA berisi data, tanpa logika sama sekali. Logikanya ada di
// main.go.
//
// Pemisahan ini disengaja: dataset akan tumbuh, dan kalau dicampur dengan kode
// penyimpanannya, main.go jadi ribuan baris yang isinya 95% teks. Dengan
// dipisah, membaca alur seeder cukup membuka satu file pendek.
//
// Catatan soal nama: semua artist, album, dan judul lagu di bawah ini FIKTIF.
// DESIGN.md §8 meminta proyek ini terlihat sebagai karya orisinal dan menghindari
// menyalin identitas milik pihak lain — memakai nama musisi asli sebagai data
// demo bertentangan dengan semangat itu, jadi seluruhnya dikarang sendiri.

type albumSeed struct {
	Title string
	Year  int
	Month time.Month
	Songs []string
}

type artistSeed struct {
	Name   string
	Bio    string
	Albums []albumSeed

	// Singles adalah lagu tanpa album — mengisi kolom album_id dengan NULL.
	//
	// Ini bukan sekadar variasi data. Song.AlbumID di model bertipe *uint
	// (nullable), dan frontend punya cabang kode khusus untuk menanganinya.
	// Tanpa satu pun baris NULL di database, cabang itu tidak akan pernah
	// tereksekusi dan bug di sana baru ketahuan saat demo.
	Singles []string
}

var artistSeeds = []artistSeed{
	{
		Name: "Senja Kolektif",
		Bio:  "Kolektif indie folk asal Yogyakarta yang menulis lagu tentang perjalanan pulang dan hal-hal kecil di antaranya.",
		Albums: []albumSeed{
			{
				Title: "Ruang Tunggu", Year: 2021, Month: time.March,
				Songs: []string{"Kereta Terakhir", "Ruang Tunggu", "Peron Tiga", "Sebelum Berangkat", "Kabar dari Utara", "Pulang Perlahan"},
			},
			{
				Title: "Halaman Belakang", Year: 2022, Month: time.September,
				Songs: []string{"Halaman Belakang", "Pohon Mangga", "Sore yang Sama", "Tetangga Lama", "Api Unggun", "Bintang Jatuh di Genteng"},
			},
			{
				Title: "Arsip Hujan", Year: 2024, Month: time.January,
				Songs: []string{"Arsip Hujan", "Payung Pinjaman", "Genangan", "Atap Seng", "Petrikor", "Sisa Gerimis", "Langit Bersih"},
			},
		},
		Singles: []string{"Lampu Jalan"},
	},
	{
		Name: "Rimba Elektrik",
		Bio:  "Duo elektronik yang meramu rekaman suara hutan dengan synthesizer modular.",
		Albums: []albumSeed{
			{
				Title: "Sinyal Hutan", Year: 2020, Month: time.June,
				Songs: []string{"Sinyal Hutan", "Kanopi", "Frekuensi Rendah", "Serangga Malam", "Embun Digital", "Akar Tembaga"},
			},
			{
				Title: "Tegangan Tinggi", Year: 2022, Month: time.February,
				Songs: []string{"Tegangan Tinggi", "Gardu Induk", "Konduktor", "Arus Balik", "Isolator", "Padam Sejenak"},
			},
			{
				Title: "Mode Pesawat", Year: 2023, Month: time.November,
				Songs: []string{"Mode Pesawat", "Ketinggian Jelajah", "Turbulensi Ringan", "Jendela 24A", "Zona Waktu Baru", "Bagasi Tertinggal"},
			},
		},
		Singles: []string{"Rekursi"},
	},
	{
		Name: "Nadia Ardhana",
		Bio:  "Penyanyi pop dengan lirik percakapan sehari-hari yang jujur dan tidak berlebihan.",
		Albums: []albumSeed{
			{
				Title: "Katanya Begitu", Year: 2021, Month: time.August,
				Songs: []string{"Katanya Begitu", "Bukan Salahmu", "Tiga Menit Lagi", "Nomor Lama", "Sebut Saja Teman", "Selesai"},
			},
			{
				Title: "Rumah Kaca", Year: 2023, Month: time.April,
				Songs: []string{"Rumah Kaca", "Tanaman Plastik", "Cahaya Pinjaman", "Retak Halus", "Suhu Ruang", "Tanpa Tirai"},
			},
			{
				Title: "Versi Lain", Year: 2025, Month: time.February,
				Songs: []string{"Versi Lain", "Kalau Saja", "Dua Puluh Tujuh", "Alamat Baru", "Kamar Sebelah", "Sampai Sini Dulu"},
			},
		},
		Singles: []string{"Nanti Kita Cerita"},
	},
	{
		Name: "Bara Timur",
		Bio:  "Band rock empat personel yang lagunya banyak bercerita tentang kerja, lelah, dan pulang.",
		Albums: []albumSeed{
			{
				Title: "Tanah Merah", Year: 2019, Month: time.October,
				Songs: []string{"Tanah Merah", "Palu dan Paku", "Jam Kerja", "Sirene Pagi", "Upah Minimum", "Pulang Larut"},
			},
			{
				Title: "Amplifier Tua", Year: 2022, Month: time.July,
				Songs: []string{"Amplifier Tua", "Senar Putus", "Panggung Kayu", "Sound Check", "Encore", "Lampu Padam"},
			},
		},
		Singles: []string{"Bensin Habis"},
	},
	{
		Name: "Laut Tenang",
		Bio:  "Proyek ambient solo dengan komposisi panjang tanpa lirik, dirancang untuk didengarkan utuh.",
		Albums: []albumSeed{
			{
				Title: "Kedalaman", Year: 2020, Month: time.December,
				Songs: []string{"Permukaan", "Sepuluh Meter", "Cahaya Terakhir", "Zona Senja", "Palung", "Dasar"},
			},
			{
				Title: "Pasang Surut", Year: 2022, Month: time.May,
				Songs: []string{"Pasang", "Bulan Purnama", "Karang", "Surut", "Jejak di Pasir", "Kembali Pasang"},
			},
			{
				Title: "Kabut Pagi", Year: 2024, Month: time.August,
				Songs: []string{"Kabut Pagi", "Perahu Kayu", "Jaring Kosong", "Matahari Malu", "Angin Darat", "Sarapan di Dermaga"},
			},
		},
	},
	{
		Name: "Gamelan Futura",
		Bio:  "Eksperimen menabrakkan laras pelog dan slendro dengan produksi elektronik kontemporer.",
		Albums: []albumSeed{
			{
				Title: "Pelog Sintetik", Year: 2021, Month: time.November,
				Songs: []string{"Pelog Sintetik", "Saron Digital", "Gong Bocor", "Kendang Patah", "Slendro Loop", "Bonang Bergema"},
			},
			{
				Title: "Ritus Baru", Year: 2023, Month: time.June,
				Songs: []string{"Ritus Baru", "Prosesi", "Sesajen Data", "Mantra Modem", "Kesurupan Halus", "Larung"},
			},
		},
		Singles: []string{"Interval Ganjil"},
	},
	{
		Name: "Reza Mahendra",
		Bio:  "Penulis lagu yang membawakan sendiri karyanya dengan gitar akustik dan sedikit alat tiup.",
		Albums: []albumSeed{
			{
				Title: "Catatan Kaki", Year: 2020, Month: time.February,
				Songs: []string{"Catatan Kaki", "Bab Tujuh", "Margin Kiri", "Kutipan", "Daftar Pustaka", "Halaman Kosong"},
			},
			{
				Title: "Sepeda Ontel", Year: 2022, Month: time.October,
				Songs: []string{"Sepeda Ontel", "Rantai Berkarat", "Turunan Panjang", "Rem Blong", "Bel Nyaring", "Parkir di Warung"},
			},
			{
				Title: "Kopi Ketiga", Year: 2024, Month: time.March,
				Songs: []string{"Kopi Ketiga", "Gula Setengah", "Barista Baru", "Meja Pojok", "Wifi Lambat", "Tutup Jam Sepuluh"},
			},
		},
		Singles: []string{"Salam dari Jauh"},
	},
	{
		Name: "Velvet Harbour",
		Bio:  "Dream pop quartet built on reverb-soaked guitars and vocals mixed just below the surface.",
		Albums: []albumSeed{
			{
				Title: "Saltwater Rooms", Year: 2021, Month: time.April,
				Songs: []string{"Saltwater Rooms", "Harbour Lights", "Slow Ferry", "Driftwood", "Low Tide Letter", "Anchor Sleep"},
			},
			{
				Title: "Paper Compass", Year: 2023, Month: time.September,
				Songs: []string{"Paper Compass", "North by Guess", "Fog Horn", "Cartographer's Regret", "Two Degrees Off", "Landfall"},
			},
			{
				Title: "Static Bloom", Year: 2025, Month: time.January,
				Songs: []string{"Static Bloom", "Velvet Static", "Radio Garden", "Hums and Halos", "Soft Interference", "Bloom Again"},
			},
		},
	},
	{
		Name: "Northern Static",
		Bio:  "Instrumental post-rock built around long crescendos and very few words.",
		Albums: []albumSeed{
			{
				Title: "Glacier Mail", Year: 2019, Month: time.May,
				Songs: []string{"Glacier Mail", "Crevasse", "Slow Thaw", "Moraine", "Blue Ice", "Meltwater"},
			},
			{
				Title: "Signal Fires", Year: 2022, Month: time.March,
				Songs: []string{"Signal Fires", "Smoke Column", "Ridge Line", "Watchtower", "Ember Field", "First Light"},
			},
		},
		Singles: []string{"Aurora Delay"},
	},
	{
		Name: "Ash & Amber",
		Bio:  "A folk duo trading verses about small towns, worn-out coats, and the long way home.",
		Albums: []albumSeed{
			{
				Title: "Two Chairs", Year: 2020, Month: time.September,
				Songs: []string{"Two Chairs", "Kitchen Table", "Borrowed Coat", "Winter Porch", "Old Dog", "Same Road Home"},
			},
			{
				Title: "Field Notes", Year: 2023, Month: time.February,
				Songs: []string{"Field Notes", "Fence Posts", "Barn Swallow", "Hay Fever", "Creek Bed", "Long Shadow"},
			},
			{
				Title: "Threadbare", Year: 2025, Month: time.April,
				Songs: []string{"Threadbare", "Mended Sleeve", "Button Jar", "Woolen Sky", "Patchwork", "Last Stitch"},
			},
		},
	},
	{
		Name: "Kaleido Bloom",
		Bio:  "Synthpop project obsessed with analog warmth and impossibly bright choruses.",
		Albums: []albumSeed{
			{
				Title: "Neon Orchard", Year: 2021, Month: time.July,
				Songs: []string{"Neon Orchard", "Plastic Peach", "Synthetic Summer", "Chrome Petals", "Sunlamp", "Harvest Loop"},
			},
			{
				Title: "Mirror Season", Year: 2024, Month: time.May,
				Songs: []string{"Mirror Season", "Reflex", "Silver Halide", "Double Exposure", "Prism Kid", "Afterimage"},
			},
		},
		Singles: []string{"Glitter Rust"},
	},
	{
		Name: "Midnight Cartography",
		Bio:  "A jazz quintet that maps cities after dark, one late set at a time.",
		Albums: []albumSeed{
			{
				Title: "Late Meridian", Year: 2020, Month: time.November,
				Songs: []string{"Late Meridian", "Third Avenue Blues", "Brass Compass", "Nocturne 4AM", "Smoke Ring Waltz", "Last Call"},
			},
			{
				Title: "Contour Lines", Year: 2022, Month: time.August,
				Songs: []string{"Contour Lines", "Elevation", "Switchback", "Valley Floor", "Summit Ridge", "Descent"},
			},
			{
				Title: "Paper Streets", Year: 2024, Month: time.October,
				Songs: []string{"Paper Streets", "Trap Street", "Grid Lock", "One Way", "Dead End Serenade", "Roundabout"},
			},
		},
	},
	{
		Name: "Paper Lanterns",
		Bio:  "Indie four-piece writing about the hours nobody else is awake for.",
		Albums: []albumSeed{
			{
				Title: "Small Hours", Year: 2021, Month: time.January,
				Songs: []string{"Small Hours", "Cheap Fireworks", "Bus Stop Bench", "Corner Shop", "Third Floor", "Sleep It Off"},
			},
			{
				Title: "Backyard Astronomy", Year: 2023, Month: time.December,
				Songs: []string{"Backyard Astronomy", "Telescope Kit", "Light Pollution", "Satellite Pass", "Meteor Excuse", "Sunrise Ruins It"},
			},
		},
		Singles: []string{"Lantern Fuel"},
	},
	{
		Name: "The Quiet Machines",
		Bio:  "Alt rock trio naming every song after something that eventually breaks.",
		Albums: []albumSeed{
			{
				Title: "Factory Settings", Year: 2019, Month: time.August,
				Songs: []string{"Factory Settings", "Hard Reset", "Idle Mode", "Overclock", "Thermal Throttle", "Safe Boot"},
			},
			{
				Title: "Analog Ghosts", Year: 2022, Month: time.April,
				Songs: []string{"Analog Ghosts", "Tape Hiss", "Reel to Reel", "Magnetic Drift", "Splice", "Rewind Forever"},
			},
			{
				Title: "Low Power", Year: 2025, Month: time.March,
				Songs: []string{"Low Power", "Battery Saver", "Dim Screen", "Airplane Heart", "Charge Cycle", "Shutdown"},
			},
		},
		Singles: []string{"Standby"},
	},
}

// userSeed adalah akun demo.
//
// Password ditulis apa adanya di sini KARENA ini data demo untuk database lokal,
// dan seeder-lah yang mem-bcrypt-nya sebelum menyimpan. Jangan pernah memakai
// email/password ini di lingkungan yang bisa diakses publik.
type userSeed struct {
	Name     string
	Email    string
	Password string
	IsAdmin  bool
}

var userSeeds = []userSeed{
	{Name: "Admin Melodia", Email: "admin@melodia.test", Password: "Admin12345!", IsAdmin: true},
	{Name: "Gabriel Baskara", Email: "gabriel@melodia.test", Password: "Password123!"},
	{Name: "Rani Kusuma", Email: "rani@melodia.test", Password: "Password123!"},
	{Name: "Dimas Prakoso", Email: "dimas@melodia.test", Password: "Password123!"},
}

// playlistSeed memilih lagu lewat RENTANG INDEKS, bukan ID.
//
// Alasannya: ID lagu baru ada setelah INSERT dijalankan, dan bisa berbeda tiap
// kali seeder dijalankan ulang tanpa -fresh. Indeks pada slice hasil pembuatan
// selalu stabil terhadap urutan data di file ini.
type playlistSeed struct {
	Name        string
	Description string
	IsPublic    bool
	// OwnerIndex menunjuk ke userSeeds di atas.
	OwnerIndex int
	// SongStride mengambil setiap lagu ke-N, mulai dari SongOffset, sebanyak SongCount.
	// Cara ini menyebarkan pilihan ke banyak artist berbeda alih-alih menumpuk
	// di satu album — playlist yang isinya satu album utuh terlihat seperti bug.
	SongOffset int
	SongStride int
	SongCount  int
}

var playlistSeeds = []playlistSeed{
	{Name: "Fokus Ngoding", Description: "Instrumental dan ambient untuk kerja panjang.", IsPublic: true, OwnerIndex: 1, SongOffset: 3, SongStride: 11, SongCount: 14},
	{Name: "Perjalanan Pulang", Description: "Teman di kereta sore.", IsPublic: true, OwnerIndex: 1, SongOffset: 0, SongStride: 7, SongCount: 16},
	{Name: "Belum Selesai", Description: "Draft, masih diseleksi.", IsPublic: false, OwnerIndex: 1, SongOffset: 5, SongStride: 17, SongCount: 9},
	{Name: "Pagi Cerah", Description: "Playlist untuk mulai hari.", IsPublic: true, OwnerIndex: 2, SongOffset: 2, SongStride: 9, SongCount: 12},
	{Name: "Late Night Drive", Description: "Long crescendos and open roads.", IsPublic: true, OwnerIndex: 3, SongOffset: 8, SongStride: 13, SongCount: 11},
	{Name: "Arsip Pribadi", Description: "Simpanan sendiri, tidak untuk dibagikan.", IsPublic: false, OwnerIndex: 3, SongOffset: 1, SongStride: 23, SongCount: 8},
}
