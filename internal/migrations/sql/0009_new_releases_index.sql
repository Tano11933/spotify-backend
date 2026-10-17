-- Index untuk endpoint rilisan terbaru (/api/albums/new-releases).
--
-- Query-nya ORDER BY release_date DESC, id DESC; index dengan urutan yang sama
-- membuat Postgres tidak perlu sort tambahan saat katalog membesar. Tie-breaker
-- id ikut di index supaya urutan album dengan tanggal rilis sama tetap stabil.

CREATE INDEX IF NOT EXISTS idx_albums_release_date ON albums (release_date DESC, id DESC);
