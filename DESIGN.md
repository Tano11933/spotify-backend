# DESIGN.md — Visual Design System (Spotify-inspired)

Dokumen ini mendefinisikan tampilan frontend agar konsisten dengan identitas visual Spotify asli. Semua nilai di bawah **wajib** dipakai lewat `tailwind.config.js` (custom theme), bukan di-hardcode di tiap komponen.

---

## 1. Color Palette

Spotify pakai dark theme sebagai basis, dengan hijau khasnya sebagai satu-satunya accent color yang mencolok — sisanya grayscale.

| Token | Hex | Penggunaan |
|---|---|---|
| `spotify-black` | `#121212` | Background utama aplikasi |
| `spotify-black-pure` | `#000000` | Background sidebar & now-playing bar |
| `spotify-dark-gray` | `#181818` | Background card (default state) |
| `spotify-elevated` | `#282828` | Background card (hover state) / elemen terangkat |
| `spotify-light-gray` | `#B3B3B3` | Teks sekunder, label, subteks |
| `spotify-white` | `#FFFFFF` | Teks utama, judul |
| `spotify-green` | `#1DB954` | Accent utama: tombol play, link aktif, indikator "sedang diputar" |
| `spotify-green-hover` | `#1ED760` | Hover state untuk elemen hijau |
| `spotify-border` | `#2A2A2A` | Border tipis antar section |

**Prinsip:** hijau (`spotify-green`) dipakai **sangat terbatas** — hanya untuk call-to-action utama (tombol Play, status aktif, link navigasi yang sedang aktif). Jangan dipakai untuk background besar atau elemen dekoratif berlebihan; ini yang membuat aksen hijau tetap terasa "istimewa" dan bukan random coloring.

```js
// tailwind.config.js — extend.colors
colors: {
  spotify: {
    black: '#121212',
    'black-pure': '#000000',
    'dark-gray': '#181818',
    elevated: '#282828',
    'light-gray': '#B3B3B3',
    white: '#FFFFFF',
    green: '#1DB954',
    'green-hover': '#1ED760',
    border: '#2A2A2A',
  },
},
```

---

## 2. Typography

Spotify pakai font custom "Circular" (proprietary, tidak boleh dipakai di luar Spotify tanpa lisensi). Untuk proyek clone, gunakan alternatif open-source dengan karakter visual serupa: **Inter** atau **Montserrat** (geometric sans-serif, clean, tersedia gratis via Google Fonts).

```js
// tailwind.config.js — extend.fontFamily
fontFamily: {
  sans: ['Inter', 'system-ui', 'sans-serif'],
}
```

| Elemen | Ukuran | Weight | Contoh penggunaan |
|---|---|---|---|
| Hero title (halaman utama) | `text-3xl` – `text-5xl` | `font-bold` (700) | "Good afternoon" greeting |
| Section heading | `text-xl` – `text-2xl` | `font-bold` (700) | "Recently played", "Made for you" |
| Card title | `text-base` | `font-semibold` (600) | Nama album/artist di card |
| Body text | `text-sm` | `font-normal` (400) | Deskripsi, metadata |
| Caption/label | `text-xs` | `font-medium` (500), `uppercase`, `tracking-wide` | Label kecil, timestamp |

---

## 3. Layout Structure

Spotify pakai layout 3-panel klasik (desktop) yang menjadi ciri khasnya:

```
┌──────────────┬─────────────────────────────────────┐
│              │  Top Bar (search, profile)           │
│   Sidebar    ├─────────────────────────────────────┤
│  (fixed, kiri)│                                      │
│              │         Main Content Area            │
│  - Home       │        (scrollable)                  │
│  - Search     │                                      │
│  - Library    │                                      │
│              │                                       │
├──────────────┴─────────────────────────────────────┤
│         Now Playing Bar (fixed, bawah)               │
└───────────────────────────────────────────────────────┘
```

### Sidebar (kiri, fixed)
- Lebar: `w-64` (desktop), collapse jadi icon-only atau hidden di mobile
- Background: `bg-spotify-black-pure`
- Isi: Logo, navigasi utama (Home, Search, Library), separator, daftar Playlist (jika ada)
- Item aktif: teks putih + background `spotify-elevated` sedikit; item non-aktif: teks `spotify-light-gray`

### Top Bar
- Background: transparan di atas, gradient fade ke `spotify-black` saat scroll (efek khas Spotify)
- Isi: tombol back/forward (opsional), search bar, avatar/menu user

### Main Content Area
- Background: `spotify-black`, dengan gradient halus di bagian atas (warna diambil dari cover album yang sedang dilihat — bisa disederhanakan jadi gradient statis untuk MVP)
- Grid card: `grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-4`

### Now Playing Bar (bawah, fixed)
- Height: `h-20` (~80px)
- Background: `bg-spotify-black-pure`, border-top `spotify-border`
- 3 kolom: info lagu (kiri), kontrol play/pause/next (tengah), volume/queue (kanan)

---

## 4. Component Patterns

### Card (Album/Artist/Song)
```
- Background: bg-spotify-dark-gray, hover: bg-spotify-elevated
- Border radius: rounded-md (8px) untuk card persegi, rounded-full untuk foto artist (bulat)
- Padding: p-4
- Transisi: transition-colors duration-200
- Cover image: aspect-square, rounded-md (album) atau rounded-full (artist)
- Tombol Play muncul saat hover: posisi absolute di pojok kanan-bawah cover,
  bg-spotify-green, rounded-full, opacity-0 group-hover:opacity-100,
  transition-opacity + translate-y-2 group-hover:translate-y-0 (efek "muncul dari bawah")
```

### Button Primary (Play, Follow, dsb)
```
- bg-spotify-green, hover:bg-spotify-green-hover
- text-spotify-black-pure (bukan putih — khas Spotify, teks di atas hijau pakai warna gelap)
- font-bold, rounded-full, px-8 py-3
- Transisi scale halus saat hover: hover:scale-105 transition-transform
```

### Button Secondary (outline)
```
- border border-spotify-light-gray, text-white
- hover: border-white, scale-105
- rounded-full, px-8 py-3
```

### Input Field (form auth)
```
- bg-spotify-elevated, border border-transparent
- focus: border-white (bukan ring biru default — sesuaikan brand)
- rounded-md, px-4 py-3, text-white
- placeholder: text-spotify-light-gray
```

### Toast Notification (untuk event WebSocket)
```
- Posisi: fixed bottom-24 right-4 (di atas Now Playing Bar)
- bg-spotify-elevated, border-l-4 border-spotify-green
- rounded-md, shadow-lg, p-4
- Animasi masuk: slide-in dari kanan + fade
```

---

## 5. Spacing & Sizing Scale

Gunakan skala default Tailwind (`4px` increment), tidak perlu custom spacing scale. Konsisten pakai:
- Padding section: `px-6 py-4` (mobile) → `px-8 py-6` (desktop)
- Gap antar card: `gap-4` (mobile) → `gap-6` (desktop)
- Border radius standar: `rounded-md` (8px) untuk card/button, `rounded-full` untuk avatar/pill button

---

## 6. Responsive Behavior

| Breakpoint | Sidebar | Grid Card | Now Playing Bar |
|---|---|---|---|
| Mobile (`< 640px`) | Hidden, ganti bottom nav bar | 2 kolom | Compact, sembunyikan kontrol tambahan |
| Tablet (`640–1024px`) | Collapse jadi icon-only | 3 kolom | Full |
| Desktop (`> 1024px`) | Full width dengan label | 4–5 kolom | Full |

---

## 7. Motion / Animasi

Ikuti prinsip "motion yang disengaja, bukan dekorasi berlebihan":
- **Hover card**: scale halus (`hover:scale-[1.02]`) + tombol play muncul dari bawah — ini signature interaction khas Spotify
- **Page transition**: fade sederhana saat pindah halaman (opsional, jangan berlebihan)
- **Now Playing Bar**: progress bar lagu berjalan smooth (`transition-all duration-1000 ease-linear` per detik)
- **Toast WebSocket**: slide-in dari kanan, auto-dismiss setelah 4 detik
- Hormati `prefers-reduced-motion` — matikan animasi non-esensial jika user mengaktifkan setting ini di OS

---

## 8. Hal yang SEBAIKNYA TIDAK ditiru 1:1

Untuk keperluan portofolio, hindari:
- Meniru logo Spotify (lingkaran hijau + garis melengkung) — pakai nama/logo buatan sendiri untuk menghindari isu trademark, meski proyek ini non-komersial
- Menyalin copy/teks marketing asli Spotify secara verbatim
- Font "Circular" asli (proprietary) — pakai Inter/Montserrat sebagai gantinya, sudah cukup mendekati kesan visualnya

Tujuannya: desain terinspirasi Spotify (familiar, dark theme, hijau signature, layout 3-panel) tapi tetap terlihat sebagai karya orisinal kamu, bukan replika identik yang berpotensi masalah trademark.
