-- Constraint & index untuk relasi follow antar user.
--
-- ON DELETE CASCADE: menghapus akun otomatis menghapus relasi follow miliknya,
-- baik sebagai pengikut maupun yang diikuti.
--
-- CHECK follower <> followee menutup follow ke diri sendiri di level database;
-- service tetap memvalidasinya lebih awal supaya pesannya ramah (422), tapi
-- aturan ini yang menjaminnya tetap benar walau ada jalur tulis lain nanti.

ALTER TABLE user_follows
  ADD CONSTRAINT fk_user_follows_follower FOREIGN KEY (follower_id) REFERENCES users (id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_user_follows_followee FOREIGN KEY (followee_id) REFERENCES users (id) ON DELETE CASCADE,
  ADD CONSTRAINT chk_user_follows_not_self CHECK (follower_id <> followee_id);

-- Daftar "siapa yang mengikuti saya" memakai kolom followee.
CREATE INDEX IF NOT EXISTS idx_user_follows_followee ON user_follows (followee_id);
