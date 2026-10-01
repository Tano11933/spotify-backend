-- Constraint & index untuk relasi artist-genre.
--
-- Tabel perantara artist_genres dibuat GORM dari tag many2many. Dua hal yang
-- tidak bisa diungkapkan lewat tag ditangani di sini:
--   1. memastikan FK memakai ON DELETE CASCADE
--   2. index balik untuk query "artist di genre X"
--
-- Constraint di-DROP lebih dulu dengan IF EXISTS. AutoMigrate GORM bisa saja
-- sudah membuat FK dengan nama yang sama (tanpa cascade), dan ADD CONSTRAINT
-- tidak punya varian IF NOT EXISTS, jadi urutan drop-lalu-add adalah cara yang
-- aman untuk kedua keadaan: tabel yang sudah punya FK maupun yang belum.

ALTER TABLE artist_genres
  DROP CONSTRAINT IF EXISTS fk_artist_genres_artist,
  DROP CONSTRAINT IF EXISTS fk_artist_genres_genre;

ALTER TABLE artist_genres
  ADD CONSTRAINT fk_artist_genres_artist FOREIGN KEY (artist_id) REFERENCES artists (id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_artist_genres_genre FOREIGN KEY (genre_id) REFERENCES genres (id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_artist_genres_genre ON artist_genres (genre_id);
