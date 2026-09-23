-- Index pencarian untuk GET /api/search.
--
-- pg_trgm membuat pencarian kemiripan (tahan typo) memakai index GIN,
-- sementara index tsvector mempercepat pencocokan kata penuh. Konfigurasi
-- 'simple' dipakai — bukan 'english' — karena stemming bahasa Inggris tidak
-- cocok untuk judul lagu berbahasa Indonesia.
--
-- IF NOT EXISTS membuat migrasi ini aman dijalankan pada database yang
-- indexnya sudah dibuat manual sebelumnya.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_songs_title_trgm ON songs USING gin (title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_songs_title_tsv ON songs USING gin (to_tsvector('simple', title));

CREATE INDEX IF NOT EXISTS idx_artists_name_trgm ON artists USING gin (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_artists_name_tsv ON artists USING gin (to_tsvector('simple', name));

CREATE INDEX IF NOT EXISTS idx_albums_title_trgm ON albums USING gin (title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_albums_title_tsv ON albums USING gin (to_tsvector('simple', title));

CREATE INDEX IF NOT EXISTS idx_playlists_name_trgm ON playlists USING gin (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_playlists_name_tsv ON playlists USING gin (to_tsvector('simple', name));
