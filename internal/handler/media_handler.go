package handler

import (
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"spotify-backend/internal/service"
	"spotify-backend/pkg/apperr"
	"spotify-backend/pkg/response"
)

const uploadFileField = "file"

type MediaHandler struct {
	service *service.MediaService
}

func NewMediaHandler(service *service.MediaService) *MediaHandler {
	return &MediaHandler{service: service}
}

// UploadAudio menerima berkas audio lewat multipart (field "file") dan
// menyimpannya untuk lagu yang dituju.
func (h *MediaHandler) UploadAudio(c *fiber.Ctx) error {
	songID, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid song id")
	}

	fileHeader, err := c.FormFile(uploadFileField)
	if err != nil {
		return response.BadRequest(c, fmt.Sprintf("multipart field %q is required", uploadFileField))
	}

	file, err := fileHeader.Open()
	if err != nil {
		return response.Internal(c)
	}
	defer func() { _ = file.Close() }()

	song, err := h.service.UploadAudio(c.UserContext(), songID, fileHeader.Filename, fileHeader.Size, file)
	if err != nil {
		return mediaError(c, err)
	}

	return c.JSON(song)
}

// Stream menyajikan berkas audio sebuah lagu, mendukung HTTP Range.
//
// Endpoint ini SENGAJA publik: elemen <audio> di browser tidak bisa memasang
// header Authorization, jadi endpoint ber-token membuat pemutaran tidak
// pernah berjalan. Trade-off yang sama diambil WebSocket lewat ?token=.
func (h *MediaHandler) Stream(c *fiber.Ctx) error {
	songID, err := parseUintParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid song id")
	}

	source, err := h.service.OpenAudio(c.UserContext(), songID)
	if err != nil {
		return mediaError(c, err)
	}

	// Tanpa berkas unggahan: alihkan ke URL eksternal (data seeder).
	if source.File == nil {
		return c.Redirect(source.Song.FileURL, fiber.StatusFound)
	}

	c.Set(fiber.HeaderContentType, service.ContentTypeForAudio(source.Song.AudioKey))

	// Berkas TIDAK ditutup di sini: fasthttp menutup body stream setelah
	// selesai menulis, dan pembungkus di serveRange menutupnya saat EOF.
	// `defer Close()` di handler justru menutup berkas sebelum body ditulis.
	return serveRange(c, source.File, source.Size)
}

// autoCloseReader membungkus berkas audio supaya descriptor-nya pasti tertutup:
// saat seluruh isi selesai dibaca (EOF), atau oleh fasthttp lewat Close().
//
// Penutupannya idempoten — fasthttp menutup body stream setelah menulis, dan
// kalau berkasnya sudah ditutup saat EOF, Close() kedua HARUS mengembalikan
// nil (bukan error "file already closed" yang akan menggagalkan response).
type autoCloseReader struct {
	file   io.ReadSeekCloser
	remain int64
	closed bool
}

func (r *autoCloseReader) close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	return r.file.Close()
}

func (r *autoCloseReader) Read(p []byte) (int, error) {
	if r.remain <= 0 {
		_ = r.close()
		return 0, io.EOF
	}

	if int64(len(p)) > r.remain {
		p = p[:r.remain]
	}

	n, err := r.file.Read(p)
	r.remain -= int64(n)

	if err == io.EOF {
		_ = r.close()
	}
	return n, err
}

func (r *autoCloseReader) Close() error {
	return r.close()
}

// serveRange melayani permintaan berkas dengan dukungan satu rentang byte
// (`Range: bytes=start-end`), yang dibutuhkan <audio> untuk seek.
//
// Tanpa header Range, seluruh berkas dikirim dengan 200; dengan Range, hanya
// potongan yang diminta dengan 206 + Content-Range.
func serveRange(c *fiber.Ctx, file io.ReadSeekCloser, size int64) error {
	c.Set(fiber.HeaderAcceptRanges, "bytes")

	start, end := int64(0), size-1

	if rangeHeader := c.Get(fiber.HeaderRange); rangeHeader != "" {
		parsedStart, parsedEnd, err := parseByteRange(rangeHeader, size)
		if err != nil {
			c.Set(fiber.HeaderContentRange, fmt.Sprintf("bytes */%d", size))
			return response.Error(c, fiber.StatusRequestedRangeNotSatisfiable, apperr.CodeValidation, "invalid range")
		}

		start, end = parsedStart, parsedEnd
		c.Status(fiber.StatusPartialContent)
		c.Set(fiber.HeaderContentRange, fmt.Sprintf("bytes %d-%d/%d", start, end, size))
	}

	if _, err := file.Seek(start, io.SeekStart); err != nil {
		_ = file.Close()
		return response.Internal(c)
	}

	length := end - start + 1
	c.Set(fiber.HeaderContentLength, strconv.FormatInt(length, 10))

	return c.SendStream(&autoCloseReader{file: file, remain: length}, int(length))
}

// parseByteRange menerjemahkan header Range menjadi pasangan start-end inklusif.
//
// Hanya satu rentang yang didukung — browser selalu meminta satu rentang, dan
// multipart/byteranges menambah kompleksitas tanpa pemakai nyata di sini.
func parseByteRange(header string, size int64) (int64, int64, error) {
	const prefix = "bytes="

	if !strings.HasPrefix(header, prefix) {
		return 0, 0, errors.New("invalid range unit")
	}

	spec := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if strings.Contains(spec, ",") {
		return 0, 0, errors.New("multiple ranges are not supported")
	}

	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return 0, 0, errors.New("invalid range")
	}

	startRaw := strings.TrimSpace(parts[0])
	endRaw := strings.TrimSpace(parts[1])

	// Bentuk suffix: "bytes=-500" → 500 byte terakhir.
	if startRaw == "" {
		suffix, err := strconv.ParseInt(endRaw, 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, errors.New("invalid range")
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, size - 1, nil
	}

	start, err := strconv.ParseInt(startRaw, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, errors.New("invalid range")
	}

	end := size - 1
	if endRaw != "" {
		parsedEnd, err := strconv.ParseInt(endRaw, 10, 64)
		if err != nil || parsedEnd < start {
			return 0, 0, errors.New("invalid range")
		}
		end = parsedEnd
		if end >= size {
			end = size - 1
		}
	}

	return start, end, nil
}

func mediaError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrSongNotFound),
		errors.Is(err, service.ErrAudioUnavailable):
		return response.NotFound(c, err.Error())

	case errors.Is(err, service.ErrEmptyAudioFile),
		errors.Is(err, service.ErrUnsupportedAudioType):
		return response.BadRequest(c, err.Error())

	case errors.Is(err, service.ErrAudioTooLarge):
		return response.Error(c, fiber.StatusRequestEntityTooLarge, apperr.CodePayloadTooLarge, err.Error())

	default:
		log.Printf("media handler error on %s %s: %v", c.Method(), c.Path(), err)
		return response.Internal(c)
	}
}
