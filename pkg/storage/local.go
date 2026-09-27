package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Local menyimpan berkas di dalam satu direktori, dengan key sebagai path
// relatif terhadap direktori itu.
type Local struct {
	baseDir string
}

func NewLocal(baseDir string) (*Local, error) {
	if strings.TrimSpace(baseDir) == "" {
		return nil, fmt.Errorf("storage: base directory is required")
	}

	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: create base directory: %w", err)
	}

	return &Local{baseDir: baseDir}, nil
}

// resolve mengubah key menjadi path absolut, sekaligus menolak key yang bisa
// keluar dari baseDir (path traversal seperti "../../etc/passwd").
func (l *Local) resolve(key string) (string, error) {
	clean := filepath.Clean(key)

	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("storage: invalid key %q", key)
	}

	return filepath.Join(l.baseDir, clean), nil
}

func (l *Local) Save(ctx context.Context, key string, r io.Reader) (int64, error) {
	path, err := l.resolve(key)
	if err != nil {
		return 0, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, fmt.Errorf("storage: create directory: %w", err)
	}

	// Tulis ke berkas sementara lalu rename: proses yang mati di tengah jalan
	// tidak meninggalkan berkas setengah jadi dengan nama final.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return 0, fmt.Errorf("storage: create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	written, err := io.Copy(tmp, r)
	if err != nil {
		_ = tmp.Close()
		return 0, fmt.Errorf("storage: write file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return 0, fmt.Errorf("storage: close temp file: %w", err)
	}

	if err := os.Rename(tmp.Name(), path); err != nil {
		return 0, fmt.Errorf("storage: move into place: %w", err)
	}

	return written, nil
}

func (l *Local) Open(ctx context.Context, key string) (io.ReadSeekCloser, int64, error) {
	path, err := l.resolve(key)
	if err != nil {
		return nil, 0, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, err
	}

	return file, info.Size(), nil
}

func (l *Local) Delete(ctx context.Context, key string) error {
	path, err := l.resolve(key)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
