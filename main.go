package main

import (
	"bright/config"
	"bright/handlers"
	"bright/ingresses"
	"bright/ingresses/postgres"
	middleware "bright/middlewares"
	"bright/raft"
	"bright/rpc"
	"bright/store"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"github.com/ansrivas/fiberprometheus/v2"
	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
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

	// Override data path if explicitly provided via flag (not default)
	if s.DataPath != "" && s.DataPath != "./data" {
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
		zap.String("data_path_from_env", os.Getenv("BRIGHT_DATA_PATH")),
		zap.String("serve_cmd_data_path", s.DataPath),
		zap.Bool("raft_enabled", cfg.RaftEnabled),
	)

	// Initialize store with configured data path
	indexStore := store.Initialize(cfg.DataPath)

	// Initialize RPC client if Raft is enabled (needed for cluster join)
	var rpcClient rpc.RPCClient
	if cfg.RaftEnabled {
		rpcClient = rpc.NewHTTPRPCClient(zapLogger)
	}

	// Initialize Raft if enabled
	var raftNode *raft.RaftNode
	if cfg.RaftEnabled {
		raftConfig := &raft.RaftConfig{
			NodeID:       cfg.RaftNodeID,
			RaftDir:      cfg.RaftDir,
			RaftBind:     cfg.RaftBind,
			RaftAdvertise: cfg.RaftAdvertise,
			Bootstrap:    cfg.RaftBootstrap,
			Peers:        cfg.GetRaftPeers(),
			MasterKey:    cfg.MasterKey,
			RPCClient:    rpcClient,
		}

		var err error
		raftNode, err = raft.NewRaftNode(raftConfig, indexStore, zapLogger)
		if err != nil {
			log.Fatal("Failed to initialize Raft:", err)
		}
		defer raftNode.Shutdown()

		zapLogger.Info("Raft enabled",
			zap.String("node_id", raftConfig.NodeID),
			zap.String("bind", raftConfig.RaftBind),
			zap.Bool("bootstrap", raftConfig.Bootstrap),
		)
	}

	// Initialize ingress manager
	ingressManager := ingresses.NewManager(cfg.DataPath, indexStore, raftNode, zapLogger)
	ingressManager.RegisterFactory("postgres", postgres.Factory)

	// Load existing ingress configurations
	if err := ingressManager.Load(); err != nil {
		zapLogger.Warn("Failed to load ingress configurations", zap.Error(err))
	}

	// Start all ingresses
	if err := ingressManager.StartAll(context.Background()); err != nil {
		zapLogger.Warn("Some ingresses failed to start", zap.Error(err))
	}
	defer ingressManager.StopAll()

	return startServer(cfg, zapLogger, indexStore, raftNode, rpcClient, ingressManager)
}

type VersionCmd struct{}

func (v *VersionCmd) Run() error {
	fmt.Printf("Bright %s\n", Version)
	return nil
}

func startServer(cfg *config.Config, zapLogger *zap.Logger, indexStore *store.IndexStore, raftNode *raft.RaftNode, rpcClient rpc.RPCClient, ingressManager *ingresses.Manager) error {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
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
	// Custom zap-based request logger
	app.Use(func(c *fiber.Ctx) error {
		start := time.Now()

		// Process request
		err := c.Next()

		// Log request
		status := c.Response().StatusCode()
		latency := time.Since(start)

		fields := []zap.Field{
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.String("ip", c.IP()),
		}

		if err != nil {
			fields = append(fields, zap.Error(err))
		}

		// Log at appropriate level based on status code
		if status >= 500 {
			zapLogger.Error("Request failed", fields...)
		} else if status >= 400 {
			zapLogger.Warn("Client error", fields...)
		} else {
			zapLogger.Info("Request completed", fields...)
		}

		return err
	})
	app.Use(recover.New())

	// Inject handler context middleware
	app.Use(func(c *fiber.Ctx) error {
		handlers.SetContext(c, &handlers.HandlerContext{
			Store:          indexStore,
			RaftNode:       raftNode,
			Config:         cfg,
			RPCClient:      rpcClient,
			IngressManager: ingressManager,
		})
		return c.Next()
	})

	// Prometheus metrics (before auth to allow scraping without authentication)
	prometheus := fiberprometheus.New("bright")
	prometheus.RegisterAt(app, "/metrics")
	app.Use(prometheus.Middleware)

	// Health check route (before auth to allow health checks without authentication)
	app.Get("/health", handlers.Health)

	// Authentication middleware
	app.Use(middleware.Authorization(cfg, zapLogger))

	// Cluster management routes (if Raft enabled)
	if cfg.RaftEnabled {
		app.Get("/cluster/status", handlers.ClusterStatus)
		app.Post("/cluster/join", handlers.JoinCluster)
	}

	// API routes grouped under /indexes
	indexes := app.Group("/indexes")
	{
		// Index management
		indexes.Get("/", handlers.ListIndexes)
		indexes.Post("/", handlers.CreateIndex)
		indexes.Get("/:id", handlers.GetIndex)
		indexes.Delete("/:id", handlers.DeleteIndex)
		indexes.Patch("/:id", handlers.UpdateIndex)

		// Document management
		indexes.Post("/:id/documents", handlers.AddDocuments)
		indexes.Delete("/:id/documents", handlers.DeleteDocuments)
		indexes.Delete("/:id/documents/:documentid", handlers.DeleteDocument)
		indexes.Patch("/:id/documents/:documentid", handlers.UpdateDocument)

		// Search
		indexes.Post("/:id/searches", handlers.Search)

		// Ingress management
		indexes.Get("/:id/ingresses", handlers.ListIngresses)
		indexes.Post("/:id/ingresses", handlers.CreateIngress)
		indexes.Get("/:id/ingresses/:ingressId", handlers.GetIngress)
		indexes.Patch("/:id/ingresses/:ingressId", handlers.UpdateIngress)
		indexes.Delete("/:id/ingresses/:ingressId", handlers.DeleteIngress)
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
