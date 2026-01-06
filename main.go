package main

import (
	"bright/config"
	"bright/handlers"
	"bright/middleware"
	"log"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// Initialize logger
	var zapLogger *zap.Logger
	if cfg.LogLevel == "debug" {
		zapLogger, err = zap.NewDevelopment()
	} else {
		zapLogger, err = zap.NewProduction()
	}
	if err != nil {
		log.Fatal("Failed to initialize logger:", err)
	}
	defer zapLogger.Sync()

	zapLogger.Info("Starting Bright",
		zap.String("port", cfg.Port),
		zap.Bool("auth_enabled", cfg.RequiresAuth()),
	)

	app := fiber.New(fiber.Config{
		AppName: "Bright",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			zapLogger.Error("Request error",
				zap.Error(err),
				zap.Int("status", code),
				zap.String("path", c.Path()),
				zap.String("method", c.Method()),
			)
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
	app.Use(middleware.Auth(cfg, zapLogger))

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
	zapLogger.Info("Server starting", zap.String("address", ":"+cfg.Port))
	if err := app.Listen(":" + cfg.Port); err != nil {
		zapLogger.Fatal("Failed to start server", zap.Error(err))
	}
}
