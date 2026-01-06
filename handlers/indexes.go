package handlers

import (
	"bright/models"
	"bright/store"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/utils"
)

// ListIndexes handles GET /indexes
func ListIndexes(c *fiber.Ctx) error {
	limit := c.QueryInt("limit", 20)
	offset := c.QueryInt("offset", 0)
	page := c.QueryInt("page", 0)

	// If page is provided, calculate offset from page
	if page > 0 {
		offset = (page - 1) * limit
	}

	s := store.GetStore()
	items := s.ListIndexes(limit, offset)

	return c.JSON(fiber.Map{
		"items": items,
	})
}

// CreateIndex handles POST /indexes
func CreateIndex(c *fiber.Ctx) error {
	id := c.Query("id")
	primaryKey := c.Query("primaryKey")

	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "id parameter is required",
		})
	}

	// Make copies of the strings to avoid Fiber buffer reuse issues
	config := &models.IndexConfig{
		ID:         utils.CopyString(id),
		PrimaryKey: utils.CopyString(primaryKey),
	}

	s := store.GetStore()
	if err := s.CreateIndex(config); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(config)
}

// DeleteIndex handles DELETE /indexes/:id
func DeleteIndex(c *fiber.Ctx) error {
	id := c.Params("id")

	s := store.GetStore()
	if err := s.DeleteIndex(id); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusNoContent).Send(nil)
}

// UpdateIndex handles PATCH /indexes/:id
func UpdateIndex(c *fiber.Ctx) error {
	id := c.Params("id")

	var config models.IndexConfig
	if err := c.BodyParser(&config); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	s := store.GetStore()
	if err := s.UpdateIndex(id, &config); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(config)
}
