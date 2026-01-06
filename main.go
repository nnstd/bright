package main

import (
	"bright/config"
	"bright/handlers"
	middleware "bright/middlewares"
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
	app.Use(middleware.Authorization(cfg, zapLogger))

	// API routes grouped under /indexes
	indexes := app.Group("/indexes")
	{
		// Index management
		indexes.Get("/", handlers.ListIndexes)
		indexes.Post("/", handlers.CreateIndex)
		indexes.Delete("/:id", handlers.DeleteIndex)
		indexes.Patch("/:id", handlers.UpdateIndex)

		// Document management
		indexes.Post("/:id/documents", handlers.AddDocuments)
		indexes.Delete("/:id/documents", handlers.DeleteDocuments)
		indexes.Delete("/:id/documents/:documentid", handlers.DeleteDocument)
		indexes.Patch("/:id/documents/:documentid", handlers.UpdateDocument)

		// Search
		indexes.Post("/:id/searches", handlers.Search)
	}

	// Start server
	zapLogger.Info("Server starting", zap.String("address", ":"+cfg.Port))
	if err := app.Listen(":" + cfg.Port); err != nil {
		zapLogger.Fatal("Failed to start server", zap.Error(err))
	}
}
