package main

import (
	"bright/handlers"
	"log"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	app := fiber.New(fiber.Config{
		AppName: "Bright",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
		JSONEncoder: sonic.Marshal,
		JSONDecoder: sonic.Unmarshal,
	})

	// Middleware
	app.Use(logger.New())
	app.Use(recover.New())

	// Index management routes
	app.Post("/indexes", handlers.CreateIndex)
	app.Delete("/indexes/:id", handlers.DeleteIndex)
	app.Patch("/indexes/:id", handlers.UpdateIndex)

	// Document management routes
	app.Post("/indexes/:id/documents", handlers.AddDocuments)
	app.Delete("/indexes/:id/documents", handlers.DeleteDocuments)
	app.Delete("/indexes/:id/documents/:documentid", handlers.DeleteDocument)
	app.Patch("/indexes/:id/documents/:documentid", handlers.UpdateDocument)

	// Search routes
	app.Post("/indexes/:id/searches", handlers.Search)

	// Start server
	log.Println("Server starting on :3000")
	if err := app.Listen(":3000"); err != nil {
		log.Fatal(err)
	}
}
