package handler

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"spotify-backend/internal/service"
	"spotify-backend/pkg/pagination"
	"spotify-backend/pkg/response"
)

type ChartHandler struct {
	service *service.ChartService
}

func NewChartHandler(service *service.ChartService) *ChartHandler {
	return &ChartHandler{service: service}
}

// TopTracks menampilkan lagu terpopuler 7 hari terakhir. Publik.
func (h *ChartHandler) TopTracks(c *fiber.Ctx) error {
	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.TopSongs(c.UserContext(), params)
	if err != nil {
		log.Printf("chart handler error on %s %s: %v", c.Method(), c.Path(), err)
		return response.Internal(c)
	}
	return c.JSON(page)
}

// TopArtists menampilkan artist terpopuler 7 hari terakhir, dihitung dari
// total putar lagu-lagunya. Publik.
func (h *ChartHandler) TopArtists(c *fiber.Ctx) error {
	params, err := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	page, err := h.service.TopArtists(c.UserContext(), params)
	if err != nil {
		log.Printf("chart handler error on %s %s: %v", c.Method(), c.Path(), err)
		return response.Internal(c)
	}
	return c.JSON(page)
}
