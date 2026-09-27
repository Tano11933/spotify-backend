//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// buildAudioMultipart menyusun body multipart berisi satu berkas audio palsu.
// Isinya bukan MP3 sungguhan — yang diuji di sini penyimpanan, validasi, dan
// pelayanan HTTP Range, bukan codec.
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

	content := bytes.Repeat([]byte("AUDIODATA"), 1000) // 9000 byte
	contentType, body := buildAudioMultipart(t, "song.mp3", content)

	status, _ := doMultipart(t, "POST", fmt.Sprintf("/api/admin/songs/%d/audio", songID), admin, contentType, body)
	if status != 200 {
		t.Fatalf("upload = %d, mau 200", status)
	}

	streamPath := fmt.Sprintf("/api/stream/songs/%d", songID)

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
	if resp.Header.Get("Content-Type") != "audio/mpeg" {
		t.Fatalf("Content-Type = %q, mau audio/mpeg", resp.Header.Get("Content-Type"))
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

	// User biasa tidak boleh mengunggah: 403.
	user := login(t, "user@test.local", "Password123!")
	status, _ = doMultipart(t, "POST", fmt.Sprintf("/api/admin/songs/%d/audio", songID), user, contentType, body)
	if status != 403 {
		t.Fatalf("upload oleh user biasa = %d, mau 403", status)
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
