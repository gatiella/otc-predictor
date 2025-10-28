package api

import (
	"fmt"
	"log"
	"os"

	"otc-predictor/internal/predictor"
	"otc-predictor/internal/storage"
	"otc-predictor/internal/tracker"
	"otc-predictor/pkg/types"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/websocket/v2"
)

// Server represents the API server
type Server struct {
	app     *fiber.App
	handler *Handler
	config  types.APIConfig
}

// NewServer creates a new API server
func NewServer(
	engine *predictor.Engine,
	storage *storage.MemoryStorage,
	tracker *tracker.ResultTracker,
	config types.APIConfig,
) *Server {
	app := fiber.New(fiber.Config{
		AppName: "OTC Predictor API",
	})

	// Middleware
	if config.EnableCORS {
		app.Use(cors.New(cors.Config{
			AllowOrigins: "*",
			AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
			AllowHeaders: "Origin, Content-Type, Accept",
		}))
	}

	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} (${latency})\n",
	}))

	handler := NewHandler(engine, storage, tracker)

	return &Server{
		app:     app,
		handler: handler,
		config:  config,
	}
}

// SetupRoutes configures all API routes
func (s *Server) SetupRoutes() {
	// API routes
	api := s.app.Group("/api")

	// Health check
	api.Get("/health", s.handler.Health)

	// Markets
	api.Get("/markets", s.handler.GetMarkets)

	// ⭐ NEW: Best trading opportunities
	api.Get("/best-markets", s.handler.GetBestMarkets)

	// Predictions
	api.Get("/predict/:market/:duration", s.handler.GetPrediction)
	api.Get("/predict/all/:duration", s.handler.GetAllPredictions)

	// Statistics
	api.Get("/stats", s.handler.GetAllStats)
	api.Get("/stats/:market", s.handler.GetStats)

	// Results
	api.Get("/results/:market", s.handler.GetResults)
	api.Get("/performance", s.handler.GetPerformanceSummary)

	// WebSocket for real-time predictions
	if s.config.WebSocketEnabled {
		api.Get("/stream/:market", websocket.New(s.handler.WebSocketHandler))
	}

	// Serve dashboard at root
	s.app.Get("/", func(c *fiber.Ctx) error {
		return c.SendFile("./web/dashboard.html")
	})

	// Serve favicon (prevent 404)
	s.app.Get("/favicon.ico", func(c *fiber.Ctx) error {
		// Check if favicon exists, otherwise return 204 No Content
		if _, err := os.Stat("./web/favicon.ico"); err == nil {
			return c.SendFile("./web/favicon.ico")
		}
		return c.SendStatus(204)
	})

	// Serve other static files if needed
	s.app.Static("/static", "./web/static")

	// 404 handler for everything else
	s.app.Use(func(c *fiber.Ctx) error {
		return c.Status(404).JSON(fiber.Map{
			"error": "Not found",
			"path":  c.Path(),
		})
	})
}

// Start starts the API server
func (s *Server) Start() error {
	s.SetupRoutes()

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	log.Printf("🌐 API Server starting on http://%s", addr)
	log.Printf("📊 Dashboard: http://%s", addr)
	log.Printf("🎯 Best Markets API: http://%s/api/best-markets", addr)
	log.Printf("📡 WebSocket: ws://%s/api/stream/:market", addr)

	return s.app.Listen(addr)
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() error {
	return s.app.Shutdown()
}
