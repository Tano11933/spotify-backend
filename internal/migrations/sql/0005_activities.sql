-- Foreign key & index untuk feed aktivitas.
--
-- ON DELETE CASCADE memastikan feed tidak menyimpan baris yang menunjuk lagu
-- atau playlist yang sudah dihapus. `song_id` dan `playlist_id` sengaja
-- nullable: satu aktivitas hanya mengisi salah satunya.

ALTER TABLE activities
  ADD CONSTRAINT fk_activities_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_activities_song FOREIGN KEY (song_id) REFERENCES songs (id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_activities_playlist FOREIGN KEY (playlist_id) REFERENCES playlists (id) ON DELETE CASCADE;

-- Feed user X = aktivitas orang yang X ikuti, terbaru dulu.
CREATE INDEX IF NOT EXISTS idx_activities_user_created ON activities (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_activities_created ON activities (created_at DESC);
