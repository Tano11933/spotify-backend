-- Materialized view agregat putar 7 hari terakhir, dasar endpoint charts.
--
-- Refresh-nya "lazy": ChartService me-refresh saat data dianggap basi (lihat
-- penanda di Redis). Worker terjadwal yang me-refresh berkala menyusul di
-- Phase E; pendekatan lazy ini membuat charts tetap benar tanpa scheduler.
--
-- Unique index dibutuhkan REFRESH ... CONCURRENTLY, yang tidak mengunci
-- pembaca selama refresh.

CREATE MATERIALIZED VIEW IF NOT EXISTS chart_song_plays_7d AS
SELECT ph.song_id, COUNT(*) AS plays
FROM play_histories ph
WHERE ph.played_at >= now() - interval '7 days'
GROUP BY ph.song_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_chart_song_plays_7d_song ON chart_song_plays_7d (song_id);
