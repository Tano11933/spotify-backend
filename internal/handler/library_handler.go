package handler

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"spotify-backend/internal/middleware"
	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/internal/service"
	"spotify-backend/pkg/pagination"
	"spotify-backend/pkg/response"
)

// maxContainsIDs membatasi jumlah id pada endpoint contains — query string
// bukan tempat menitipkan ratusan id; frontend memanggilnya per layar.
const maxContainsIDs = 100

type LibraryHandler struct {
	service *service.LibraryService
}

func NewLibraryHandler(service *service.LibraryService) *LibraryHandler {
	return &LibraryHandler{service: service}
}

/* -------------------------------------------------------------------------
 * Liked songs
 * ---------------------------------------------------------------------- */

func (h *LibraryHandler) SaveTrack(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	songID, err := parseUintParam(c, "songId")
	if err != nil {
		return response.BadRequest(c, "invalid song id")
	}

	if err := h.service.SaveTrack(c.UserContext(), userID, songID); err != nil {
		return libraryError(c, err)
	}

	return c.JSON(model.MessageResponse{Message: "song saved to library"})
}

func (h *LibraryHandler) RemoveTrack(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	songID, err := parseUintParam(c, "songId")
	if err != nil {
		return response.BadRequest(c, "invalid song id")
	}

	if err := h.service.RemoveTrack(c.UserContext(), userID, songID); err != nil {
		return libraryError(c, err)
	}

	return c.JSON(model.MessageResponse{Message: "song removed from library"})
}

func (h *LibraryHandler) GetTracks(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetTracks(c.UserContext(), userID, params)
	if err != nil {
		return libraryError(c, err)
	}
	return c.JSON(page)
}

func (h *LibraryHandler) TracksContain(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	ids, err := parseIDList(c.Query("ids"), maxContainsIDs)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	contains, err := h.service.TracksContain(c.UserContext(), userID, ids)
	if err != nil {
		return libraryError(c, err)
	}
	return c.JSON(contains)
}

/* -------------------------------------------------------------------------
 * Saved albums
 * ---------------------------------------------------------------------- */

func (h *LibraryHandler) SaveAlbum(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	albumID, err := parseUintParam(c, "albumId")
	if err != nil {
		return response.BadRequest(c, "invalid album id")
	}

	if err := h.service.SaveAlbum(c.UserContext(), userID, albumID); err != nil {
		return libraryError(c, err)
	}

	return c.JSON(model.MessageResponse{Message: "album saved to library"})
}

func (h *LibraryHandler) RemoveAlbum(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	albumID, err := parseUintParam(c, "albumId")
	if err != nil {
		return response.BadRequest(c, "invalid album id")
	}

	if err := h.service.RemoveAlbum(c.UserContext(), userID, albumID); err != nil {
		return libraryError(c, err)
	}

	return c.JSON(model.MessageResponse{Message: "album removed from library"})
}

func (h *LibraryHandler) GetAlbums(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetAlbums(c.UserContext(), userID, params)
	if err != nil {
		return libraryError(c, err)
	}
	return c.JSON(page)
}

func (h *LibraryHandler) AlbumsContain(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	ids, err := parseIDList(c.Query("ids"), maxContainsIDs)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	contains, err := h.service.AlbumsContain(c.UserContext(), userID, ids)
	if err != nil {
		return libraryError(c, err)
	}
	return c.JSON(contains)
}

/* -------------------------------------------------------------------------
 * Followed artists
 * ---------------------------------------------------------------------- */

func (h *LibraryHandler) FollowArtist(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	artistID, err := parseUintParam(c, "artistId")
	if err != nil {
		return response.BadRequest(c, "invalid artist id")
	}

	if err := h.service.FollowArtist(c.UserContext(), userID, artistID); err != nil {
		return libraryError(c, err)
	}

	return c.JSON(model.MessageResponse{Message: "artist followed"})
}

func (h *LibraryHandler) UnfollowArtist(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	artistID, err := parseUintParam(c, "artistId")
	if err != nil {
		return response.BadRequest(c, "invalid artist id")
	}

	if err := h.service.UnfollowArtist(c.UserContext(), userID, artistID); err != nil {
		return libraryError(c, err)
	}

	return c.JSON(model.MessageResponse{Message: "artist unfollowed"})
}

func (h *LibraryHandler) GetFollowing(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.GetFollowing(c.UserContext(), userID, params)
	if err != nil {
		return libraryError(c, err)
	}
	return c.JSON(page)
}

func (h *LibraryHandler) FollowingContain(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		return response.Unauthorized(c, "unauthorized")
	}

	ids, err := parseIDList(c.Query("ids"), maxContainsIDs)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	contains, err := h.service.FollowingContain(c.UserContext(), userID, ids)
	if err != nil {
		return libraryError(c, err)
	}
	return c.JSON(contains)
}

/* -------------------------------------------------------------------------
 * Helper
 * ---------------------------------------------------------------------- */

// parseIDList membaca "1,2,3" menjadi slice uint tanpa duplikat.
func parseIDList(raw string, max int) ([]uint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("ids is required")
	}

	seen := make(map[uint]bool)
	ids := make([]uint, 0, 8)

	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		value, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return nil, errors.New("ids must be comma-separated numbers")
		}

		id := uint(value)
		if seen[id] {
			continue
		}

		seen[id] = true
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return nil, errors.New("ids is required")
	}
	if len(ids) > max {
		return nil, fmt.Errorf("too many ids (max %d)", max)
	}

	return ids, nil
}

// libraryError memetakan error library. Berbeda dari respondError katalog,
// artist yang tidak ditemukan di sini adalah 404 — konteksnya "menyimpan/
// mengikuti sesuatu yang tidak ada", bukan "membuat album dengan artist salah".
func libraryError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, repository.ErrNotFound),
		errors.Is(err, service.ErrSongNotFound),
		errors.Is(err, service.ErrAlbumNotFound),
		errors.Is(err, service.ErrArtistNotFound):
		return response.NotFound(c, err.Error())

	default:
		log.Printf("library handler error on %s %s: %v", c.Method(), c.Path(), err)
		return response.Internal(c)
	}
}
