# Backup dan restore Melodia

Untuk stack `docker-compose.prod.yml`, simpan dump PostgreSQL dan arsip `uploads_data` sebagai satu pasangan. PostgreSQL menyimpan akun, katalog, playlist, riwayat, antrean lagu, dan posisi pemutaran. Volume audio menyimpan isi berkas yang dirujuk katalog. Simpan pasangan backup di luar repositori dan batasi aksesnya karena dump berisi data akun.

Contoh PowerShell ini dijalankan dari root backend. Ganti nama project sumber jika stack memakai `-p`. Gunakan Compose dan konfigurasi database yang sama dengan stack sumber.

## Backup

Hentikan API, worker, serta proses lain yang menulis ke database atau volume audio. PostgreSQL dan Redis tetap berjalan. Blok `finally` menyalakan kembali API dan worker jika langkah backup gagal.

```powershell
$sourceProject = 'spotify-backend-prod'
$envFile = (Resolve-Path .env).Path
$backup = Join-Path ([IO.Path]::GetTempPath()) ('melodia-backup-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $backup | Out-Null

function Source-Compose {
  docker compose --env-file $envFile -p $sourceProject -f docker-compose.prod.yml @args
  if ($LASTEXITCODE -ne 0) { throw 'Perintah Compose sumber gagal.' }
}

try {
  Source-Compose stop api worker
  Source-Compose exec -T postgres sh -ec 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc -f /tmp/melodia.dump'
  Source-Compose cp postgres:/tmp/melodia.dump (Join-Path $backup 'melodia.dump')
  Source-Compose exec -T postgres rm -f /tmp/melodia.dump
  Source-Compose run --rm --no-deps -T --user 0 --volume "${backup}:/backup" api sh -ec 'tar -czf /backup/uploads.tar.gz -C /app/uploads .'
  Get-FileHash (Join-Path $backup 'melodia.dump'), (Join-Path $backup 'uploads.tar.gz')
} finally {
  Source-Compose up -d --wait api worker
}
$backup
```

`pg_dump -Fc` menulis archive yang dibaca oleh `pg_restore`. `compose cp` memindahkan berkas biner ke host dan menghindari perbedaan redirection PowerShell. Catat hasil hash dan SHA source bersama backup. [Dokumentasi pg_dump](https://www.postgresql.org/docs/16/app-pgdump.html)

## Restore ke project baru

Gunakan pasangan backup yang tepercaya. Pakai PostgreSQL dengan major version yang sama dan checkout aplikasi yang sesuai dengan dump. Buat project Compose baru dengan volume kosong. API dan worker baru boleh menyala setelah database dan audio selesai dipulihkan.

Jalankan dari sesi PowerShell yang sama, atau isi kembali `$backup` dan `$envFile`. Contoh ini menggunakan port host 19091. Jika frontend ikut diarahkan ke hasil restore, samakan origin CORS di backend dan build ulang frontend dengan URL API dan WebSocket yang baru.

```powershell
$restoreProject = 'melodia-restore-' + [guid]::NewGuid().ToString('N').Substring(0, 8)
$previousAppPort = $env:APP_PORT
$env:APP_PORT = '19091'

function Restore-Compose {
  docker compose --env-file $envFile -p $restoreProject -f docker-compose.prod.yml @args
  if ($LASTEXITCODE -ne 0) { throw 'Perintah Compose restore gagal.' }
}

Restore-Compose build api worker
Restore-Compose up -d --wait postgres redis
Restore-Compose cp (Join-Path $backup 'melodia.dump') postgres:/tmp/melodia.dump
Restore-Compose exec -T postgres sh -ec 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --single-transaction --no-owner --no-privileges /tmp/melodia.dump'
Restore-Compose exec -T postgres rm -f /tmp/melodia.dump
Restore-Compose run --rm --no-deps -T --user 0 --volume "${backup}:/backup:ro" api sh -ec 'test -z "$(ls -A /app/uploads)" && tar -xzf /backup/uploads.tar.gz -C /app/uploads && chown -R app:app /app/uploads'
Restore-Compose up -d --wait
```

Restore database memakai satu transaksi. Jika restore database atau ekstraksi audio gagal, perbaiki penyebabnya dan ulangi di project baru dengan volume kosong. [Dokumentasi pg_restore](https://www.postgresql.org/docs/16/app-pgrestore.html)

## Smoke check

1. Panggil `http://127.0.0.1:19091/ready` dan pastikan HTTP 200.
2. Buka katalog, masuk dengan akun yang sudah ada di dump, dan periksa playlist atau data yang mudah dikenali.
3. Gunakan ID lagu yang audionya diunggah sebelum backup. Panggil endpoint dengan Range dan pastikan HTTP 206. Unduh stream penuh dan bandingkan SHA-256 dengan berkas asli.

```powershell
$songId = [uint32](Read-Host 'ID lagu yang audionya sudah diunggah sebelum backup')
$url = "http://127.0.0.1:19091/api/stream/songs/$songId"
$status = curl.exe -fsS -H 'Range: bytes=0-255' -o NUL -w '%{http_code}' $url
if ($LASTEXITCODE -ne 0 -or $status -ne '206') { throw 'Stream restore gagal menjawab HTTP 206.' }
curl.exe -fsS -o (Join-Path $backup 'restored.wav') $url
if ($LASTEXITCODE -ne 0) { throw 'Unduhan stream restore gagal.' }
Get-FileHash (Join-Path $backup 'restored.wav')
Restore-Compose logs --tail 30 worker
Restore-Compose down
$env:APP_PORT = $previousAppPort
```

Perintah `down` mempertahankan volume hasil restore. Hapus volume hanya jika project sementara sudah selesai diverifikasi dan datanya boleh dibuang.

Prosedur ini diuji dengan dua project Docker Compose sementara. Hasil restore
lolos readiness, login admin, dan pemeriksaan katalog. Stream menjawab HTTP 206
dan SHA-256 WAV hasil restore sama dengan WAV sebelum backup. Worker juga
menyelesaikan job refresh chart setelah restore.

## Jika Redis hilang

Compose demo menggunakan Redis tanpa named volume. Restore PostgreSQL dan audio mempertahankan data di kedua sumber itu, termasuk antrean lagu pengguna. Cache Redis terisi ulang. Refresh session dan tautan reset lama tidak bisa dipakai, jadi pengguna perlu login lagi atau meminta tautan baru. Access token yang masih valid tetap mengikuti TTL dan konfigurasi signing key. Job worker yang tersimpan di Redis, termasuk retry dan dead-letter, perlu ditinjau atau dijadwalkan ulang dari sumber yang masih tersedia. Gunakan catatan pengiriman sebelum mencoba ulang email agar tidak mengirim ulang pesan yang sudah diterima. [Siklus sesi](login-sessions.md) · [Pemulihan job](job-recovery.md)
