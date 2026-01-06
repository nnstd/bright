package main

import (
	"bright/config"
	"bright/handlers"
	middleware "bright/middlewares"
	"bright/store"
	"fmt"
	"log"
	"os"

	"github.com/alecthomas/kong"
	"github.com/ansrivas/fiberprometheus/v2"
	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"
)

// Version is set via ldflags during build
var Version = "dev"

var CLI struct {
	Serve   ServeCmd   `cmd:"" help:"Start the Bright server" default:"1"`
	Version VersionCmd `cmd:"" help:"Show version information"`
}

type ServeCmd struct {
	MasterKey string `help:"Master key for authentication (overrides BRIGHT_MASTER_KEY env var)" env:"BRIGHT_MASTER_KEY"`
	DataPath  string `help:"Path to data directory (overrides DATA_PATH env var)" env:"DATA_PATH" default:"./data"`
}

func (s *ServeCmd) Run() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// Override master key if provided via flag
	if s.MasterKey != "" {
		cfg.MasterKey = s.MasterKey
	}

	// Override data path if provided via flag
	if s.DataPath != "" {
		cfg.DataPath = s.DataPath
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
		zap.String("data_path", cfg.DataPath),
	)

	// Initialize store with configured data path
	store.Initialize(cfg.DataPath)

	return startServer(cfg, zapLogger)
}

type VersionCmd struct{}

func (v *VersionCmd) Run() error {
	fmt.Printf("Bright %s\n", Version)
	return nil
}

func startServer(cfg *config.Config, zapLogger *zap.Logger) error {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: false,
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

	// Prometheus metrics (before auth to allow scraping without authentication)
	prometheus := fiberprometheus.New("bright")
	prometheus.RegisterAt(app, "/metrics")
	app.Use(prometheus.Middleware)

	// Authentication middleware
	app.Use(middleware.Authorization(cfg, zapLogger))

	// Health check route
	app.Get("/health", handlers.Health)

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
		return err
	}
	return nil
}

func main() {
	ctx := kong.Parse(&CLI,
		kong.Name("bright"),
		kong.Description("A blazing fast full-text search server"),
		kong.UsageOnError(),
	)
	err := ctx.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
