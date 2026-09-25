-- Foreign key & index untuk library user (saved_tracks, saved_albums,
-- followed_artists).
--
-- AutoMigrate sudah membuat tabel beserta composite primary key-nya; file ini
-- menambahkan yang tidak bisa diungkapkan lewat struct tag GORM tanpa
-- mendeklarasikan association:
--
--   ON DELETE CASCADE  -> menghapus lagu/album/artist otomatis membersihkan
--                         entri library semua user (perilaku Spotify).
--   index (user_id, saved_at DESC) -> mempercepat daftar "terbaru disimpan
--                         lebih dulu" tanpa sort tambahan.
--
-- ADD CONSTRAINT tidak punya bentuk IF NOT EXISTS di Postgres; keamanan
-- re-run dijaga tabel schema_migrations milik runner migrasi.

ALTER TABLE saved_tracks
  ADD CONSTRAINT fk_saved_tracks_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_saved_tracks_song FOREIGN KEY (song_id) REFERENCES songs (id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_saved_tracks_user_saved_at ON saved_tracks (user_id, saved_at DESC);

ALTER TABLE saved_albums
  ADD CONSTRAINT fk_saved_albums_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_saved_albums_album FOREIGN KEY (album_id) REFERENCES albums (id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_saved_albums_user_saved_at ON saved_albums (user_id, saved_at DESC);

ALTER TABLE followed_artists
  ADD CONSTRAINT fk_followed_artists_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_followed_artists_artist FOREIGN KEY (artist_id) REFERENCES artists (id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_followed_artists_user_followed_at ON followed_artists (user_id, followed_at DESC);
