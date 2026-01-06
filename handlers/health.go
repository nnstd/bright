package handlers

import "github.com/gofiber/fiber/v2"

// Health handles GET /health
func Health(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "ok",
	})
}
