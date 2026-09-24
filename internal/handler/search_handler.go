package handler

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"

	"spotify-backend/internal/service"
	"spotify-backend/pkg/pagination"
	"spotify-backend/pkg/response"
)

const minSearchQueryLength = 2

// searchTypes adalah tipe yang boleh diminta lewat ?type=.
var searchTypes = map[string]bool{
	"track":    true,
	"artist":   true,
	"album":    true,
	"playlist": true,
}

type SearchHandler struct {
	service *service.SearchService
}

func NewSearchHandler(service *service.SearchService) *SearchHandler {
	return &SearchHandler{service: service}
}

// Search menangani GET /api/search?q=&type=&limit=&offset=.
func (h *SearchHandler) Search(c *fiber.Ctx) error {
	term := strings.TrimSpace(c.Query("q"))
	if utf8.RuneCountInString(term) < minSearchQueryLength {
		return response.BadRequest(c, "query must be at least 2 characters")
	}

	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	types, err := parseSearchTypes(c.Query("type"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	results, err := h.service.Search(c.UserContext(), term, types, params)
	if err != nil {
		return response.Internal(c)
	}

	return c.JSON(results)
}

// parseSearchTypes membaca daftar tipe yang dipisah koma. Kosong berarti semua
// tipe; tipe yang tidak dikenal ditolak supaya typo tidak diam-diam mengembalikan
// hasil kosong.
func parseSearchTypes(raw string) (map[string]bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]bool{
			"track": true, "artist": true, "album": true, "playlist": true,
		}, nil
	}

	types := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if !searchTypes[name] {
			return nil, errors.New("unknown search type: " + name)
		}
		types[name] = true
	}

	if len(types) == 0 {
		return nil, errors.New("no search type requested")
	}

	return types, nil
}
