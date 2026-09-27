-- Foreign key & index untuk fitur playback: player_states, queue_items,
-- play_histories.
--
-- AutoMigrate sudah membuat tabelnya; file ini menambahkan ON DELETE CASCADE
-- (menghapus lagu/user otomatis membersihkan state, antrean, dan riwayat
-- miliknya) plus index yang dipakai query harian.

ALTER TABLE player_states
  ADD CONSTRAINT fk_player_states_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_player_states_song FOREIGN KEY (song_id) REFERENCES songs (id) ON DELETE CASCADE;

ALTER TABLE queue_items
  ADD CONSTRAINT fk_queue_items_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_queue_items_song FOREIGN KEY (song_id) REFERENCES songs (id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_queue_items_user_position ON queue_items (user_id, position);

ALTER TABLE play_histories
  ADD CONSTRAINT fk_play_histories_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_play_histories_song FOREIGN KEY (song_id) REFERENCES songs (id) ON DELETE CASCADE;

-- Riwayat selalu dibaca "milik user X, terbaru dulu".
CREATE INDEX IF NOT EXISTS idx_play_histories_user_played_at ON play_histories (user_id, played_at DESC);
