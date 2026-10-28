//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/internal/service"
	"spotify-backend/pkg/storage"
)

// buildAudioMultipart builds one file part without relying on its MIME header.
func buildAudioMultipart(t *testing.T, filename string, content []byte) (string, []byte) {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile(uploadFieldForTest, filename)
	if err != nil {
		t.Fatalf("buat form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("tulis isi berkas: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("tutup multipart writer: %v", err)
	}

	return writer.FormDataContentType(), buf.Bytes()
}

const uploadFieldForTest = "file"

// doMultipart mengirim request multipart ke aplikasi Fiber (in-memory).
func doMultipart(t *testing.T, method, path, token, contentType string, body []byte) (int, map[string]any) {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := testApp.Fiber.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var payload map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &payload)
	}
	return resp.StatusCode, payload
}

// fetchRaw mengambil response mentah — dibutuhkan untuk memeriksa status 206
// dan isi byte, bukan JSON.
func fetchRaw(t *testing.T, path string, headers map[string]string) (*http.Response, []byte) {
	t.Helper()

	req := httptest.NewRequest("GET", path, nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := testApp.Fiber.Test(req, -1)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

// TestAudioUploadAndStream mengunci alur media: unggah oleh admin, streaming
// penuh, HTTP Range (potongan & suffix), range tidak valid, format ditolak,
// dan guard admin.
func TestAudioUploadAndStream(t *testing.T) {
	admin := login(t, "admin@test.local", "Password123!")

	_, created := do(t, "POST", "/api/songs", admin, map[string]any{
		"title": "Media Temp Song", "duration": 60, "artist_id": fixtureArtistID,
	})
	songID := uint(created["id"].(float64))
	t.Cleanup(func() {
		do(t, "DELETE", fmt.Sprintf("/api/songs/%d", songID), admin, nil)
	})

	content := pcmFixture(9000)
	contentType, body := buildAudioMultipart(t, "song.wav", content)

	status, _ := doMultipart(t, "POST", fmt.Sprintf("/api/admin/songs/%d/audio", songID), admin, contentType, body)
	if status != 200 {
		t.Fatalf("upload = %d, mau 200", status)
	}

	streamPath := fmt.Sprintf("/api/stream/songs/%d", songID)
	largeType, largeBody := buildAudioMultipart(t, "large.wav", make([]byte, 20*1024*1024+1))
	status, _ = doMultipart(t, "POST", fmt.Sprintf("/api/admin/songs/%d/audio", songID), admin, largeType, largeBody)
	if status != 413 {
		t.Fatalf("oversized upload = %d, want 413", status)
	}

	// Tanpa Range: 200 + isi utuh.
	resp, got := fetchRaw(t, streamPath, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("stream penuh = %d, mau 200", resp.StatusCode)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("isi stream tidak sama: %d byte, mau %d", len(got), len(content))
	}
	if resp.Header.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("Accept-Ranges = %q", resp.Header.Get("Accept-Ranges"))
	}
	if resp.Header.Get("Content-Type") != "audio/wav" {
		t.Fatalf("Content-Type = %q, mau audio/wav", resp.Header.Get("Content-Type"))
	}

	// Range eksplisit: 206 + potongan yang benar.
	resp, got = fetchRaw(t, streamPath, map[string]string{"Range": "bytes=10-19"})
	if resp.StatusCode != 206 {
		t.Fatalf("range = %d, mau 206", resp.StatusCode)
	}
	if want := fmt.Sprintf("bytes 10-19/%d", len(content)); resp.Header.Get("Content-Range") != want {
		t.Fatalf("Content-Range = %q, mau %q", resp.Header.Get("Content-Range"), want)
	}
	if !bytes.Equal(got, content[10:20]) {
		t.Fatalf("potongan range salah: %q", got)
	}

	// Suffix range: 100 byte terakhir.
	resp, got = fetchRaw(t, streamPath, map[string]string{"Range": "bytes=-100"})
	if resp.StatusCode != 206 {
		t.Fatalf("suffix range = %d, mau 206", resp.StatusCode)
	}
	if !bytes.Equal(got, content[len(content)-100:]) {
		t.Fatalf("suffix range salah: %d byte", len(got))
	}

	// Range di luar berkas: 416.
	resp, _ = fetchRaw(t, streamPath, map[string]string{"Range": "bytes=999999-"})
	if resp.StatusCode != 416 {
		t.Fatalf("range di luar berkas = %d, mau 416", resp.StatusCode)
	}

	// Format tidak didukung: 400.
	badType, badBody := buildAudioMultipart(t, "song.txt", content)
	status, _ = doMultipart(t, "POST", fmt.Sprintf("/api/admin/songs/%d/audio", songID), admin, badType, badBody)
	if status != 400 {
		t.Fatalf("upload .txt = %d, mau 400", status)
	}
	for _, invalid := range []struct {
		name string
		data []byte
	}{
		{"empty.wav", nil}, {"fake.mp3", []byte("not audio")}, {"wrong.mp3", content},
		{"truncated.wav", content[:50]}, {"video.m4a", content},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			ct, b := buildAudioMultipart(t, invalid.name, invalid.data)
			status, _ := doMultipart(t, "POST", fmt.Sprintf("/api/admin/songs/%d/audio", songID), admin, ct, b)
			if status != 400 {
				t.Fatalf("invalid upload = %d", status)
			}
			_, got := fetchRaw(t, streamPath, nil)
			if !bytes.Equal(got, content) {
				t.Fatal("failed upload replaced old audio")
			}
		})
	}
	replacement := pcmFixture(4000)
	ct, b := buildAudioMultipart(t, "replacement.WAV", replacement)
	status, _ = doMultipart(t, "POST", fmt.Sprintf("/api/admin/songs/%d/audio", songID), admin, ct, b)
	if status != 200 {
		t.Fatalf("replacement = %d", status)
	}
	resp, got = fetchRaw(t, streamPath, map[string]string{"Range": "bytes=40-59"})
	if resp.StatusCode != 206 || !bytes.Equal(got, replacement[40:60]) {
		t.Fatal("replacement range mismatch")
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("replacement may be cached")
	}
	status, _ = doMultipart(t, "POST", fmt.Sprintf("/api/admin/songs/%d/audio", songID), "", ct, b)
	if status != 401 {
		t.Fatalf("anonymous upload = %d", status)
	}

	// User biasa tidak boleh mengunggah: 403.
	user := login(t, "user@test.local", "Password123!")
	status, _ = doMultipart(t, "POST", fmt.Sprintf("/api/admin/songs/%d/audio", songID), user, contentType, body)
	if status != 403 {
		t.Fatalf("upload oleh user biasa = %d, mau 403", status)
	}
}

func pcmFixture(samples int) []byte {
	data := make([]byte, 44+samples*2)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 8000)
	binary.LittleEndian.PutUint32(data[28:], 16000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(samples*2))
	return data
}

func TestAudioSizeAndReplacementCleanup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := storage.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	song := model.Song{Title: "Size fixture", Duration: 1, ArtistID: fixtureArtistID}
	if err := db.Create(&song).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Delete(&song) })
	svc := service.NewMediaService(repository.NewSongRepository(db), store, 1024)
	valid := pcmFixture(200)
	first, err := svc.UploadAudio(ctx, song.ID, "first.wav", int64(len(valid)), bytes.NewReader(valid))
	if err != nil {
		t.Fatal(err)
	}
	oldKey := first.AudioKey
	for _, declared := range []int64{1, 2048} {
		_, err = svc.UploadAudio(ctx, song.ID, "large.wav", declared, bytes.NewReader(pcmFixture(1000)))
		if !errors.Is(err, service.ErrAudioTooLarge) {
			t.Fatalf("size error = %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, oldKey)); err != nil {
		t.Fatal("old audio lost", err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "songs", "*"))
	if len(files) != 1 {
		t.Fatalf("failed uploads left %d files", len(files))
	}
	second, err := svc.UploadAudio(ctx, song.ID, "second.wav", int64(len(valid)), bytes.NewReader(valid))
	if err != nil {
		t.Fatal(err)
	}
	if second.AudioKey == oldKey {
		t.Fatal("replacement reused storage key")
	}
	if _, err := os.Stat(filepath.Join(dir, oldKey)); !os.IsNotExist(err) {
		t.Fatal("old file not removed", err)
	}
	files, _ = filepath.Glob(filepath.Join(dir, "songs", "*"))
	if len(files) != 1 {
		t.Fatalf("replacement left %d files", len(files))
	}
	results := make(chan error, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			_, err := svc.UploadAudio(ctx, song.ID, "concurrent.wav", int64(len(valid)), bytes.NewReader(valid))
			results <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	files, _ = filepath.Glob(filepath.Join(dir, "songs", "*"))
	if len(files) != 1 {
		t.Fatalf("concurrent replacements left %d files", len(files))
	}
	source, err := svc.OpenAudio(ctx, song.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer source.File.Close()
	got, err := io.ReadAll(source.File)
	if err != nil || !bytes.Equal(got, valid) {
		t.Fatal("concurrent replacement lost audio", err)
	}
}

// TestAudioStreamFallbackRedirect mengunci jalur data seeder: lagu tanpa
// berkas unggahan dialihkan ke file_url eksternalnya.
func TestAudioStreamFallbackRedirect(t *testing.T) {
	admin := login(t, "admin@test.local", "Password123!")

	_, created := do(t, "POST", "/api/songs", admin, map[string]any{
		"title":     "Redirect Temp Song",
		"duration":  30,
		"artist_id": fixtureArtistID,
		"file_url":  "https://example.com/temp.mp3",
	})
	songID := uint(created["id"].(float64))
	t.Cleanup(func() {
		do(t, "DELETE", fmt.Sprintf("/api/songs/%d", songID), admin, nil)
	})

	resp, _ := fetchRaw(t, fmt.Sprintf("/api/stream/songs/%d", songID), nil)
	if resp.StatusCode != 302 {
		t.Fatalf("fallback = %d, mau 302", resp.StatusCode)
	}
	if location := resp.Header.Get("Location"); location != "https://example.com/temp.mp3" {
		t.Fatalf("Location = %q", location)
	}

	// Lagu tak dikenal → 404.
	resp, _ = fetchRaw(t, "/api/stream/songs/999999", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("stream lagu tak dikenal = %d, mau 404", resp.StatusCode)
	}
}
