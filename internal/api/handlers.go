package api

import (
	"log"
	"strconv"
	"time"

	"otc-predictor/internal/predictor"
	"otc-predictor/internal/storage"
	"otc-predictor/internal/tracker"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

// Handler handles HTTP requests
type Handler struct {
	engine  *predictor.Engine
	storage *storage.MemoryStorage
	tracker *tracker.ResultTracker
}

// NewHandler creates a new API handler
func NewHandler(engine *predictor.Engine, storage *storage.MemoryStorage, tracker *tracker.ResultTracker) *Handler {
	return &Handler{
		engine:  engine,
		storage: storage,
		tracker: tracker,
	}
}

// GetPrediction handles GET /predict/:market/:duration
func (h *Handler) GetPrediction(c *fiber.Ctx) error {
	market := c.Params("market")
	durationStr := c.Params("duration")

	duration, err := strconv.Atoi(durationStr)
	if err != nil || duration < 30 || duration > 900 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid duration (must be between 30-900 seconds)",
		})
	}

	prediction, err := h.engine.Predict(market, duration)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(prediction)
}

// GetAllPredictions handles GET /predict/all/:duration
func (h *Handler) GetAllPredictions(c *fiber.Ctx) error {
	durationStr := c.Params("duration")

	duration, err := strconv.Atoi(durationStr)
	if err != nil || duration < 30 || duration > 900 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid duration (must be between 30-900 seconds)",
		})
	}

	predictions := h.engine.PredictAll(duration)
	return c.JSON(predictions)
}

// GetStats handles GET /stats/:market
func (h *Handler) GetStats(c *fiber.Ctx) error {
	market := c.Params("market")
	stats := h.engine.GetStats(market)
	return c.JSON(stats)
}

// GetAllStats handles GET /stats
func (h *Handler) GetAllStats(c *fiber.Ctx) error {
	stats := h.engine.GetAllStats()
	return c.JSON(stats)
}

// GetMarkets handles GET /markets
func (h *Handler) GetMarkets(c *fiber.Ctx) error {
	markets := h.storage.GetActiveMarkets()
	response := make([]fiber.Map, len(markets))

	for i, market := range markets {
		tickCount := h.storage.GetTickCount(market)
		latestPrice := h.storage.GetLatestPrice(market)

		response[i] = fiber.Map{
			"market":       market,
			"tick_count":   tickCount,
			"latest_price": latestPrice,
			"active":       tickCount > 0,
		}
	}

	return c.JSON(response)
}

// Health handles GET /health
func (h *Handler) Health(c *fiber.Ctx) error {
	markets := h.storage.GetActiveMarkets()

	return c.JSON(fiber.Map{
		"status":         "ok",
		"timestamp":      time.Now(),
		"active_markets": len(markets),
		"markets":        markets,
	})
}

// WebSocketHandler handles WebSocket connections for real-time predictions
// OPTIMIZED FOR QUALITY: Updates every 12 seconds (5 requests/minute max)
func (h *Handler) WebSocketHandler(c *websocket.Conn) {
	market := c.Params("market")
	durationStr := c.Query("duration", "60")

	duration, err := strconv.Atoi(durationStr)
	if err != nil || duration < 30 || duration > 900 {
		duration = 60
	}

	log.Printf("📡 WebSocket connected: %s (duration: %ds, quality mode)", market, duration)

	defer func() {
		c.Close()
		log.Printf("📡 WebSocket disconnected: %s", market)
	}()

	// Send predictions every 12 seconds (quality mode - respects rate limit)
	// This ensures 5 predictions per minute, matching the rate limit
	ticker := time.NewTicker(12 * time.Second)
	defer ticker.Stop()

	// Send initial prediction immediately
	prediction, err := h.engine.Predict(market, duration)
	if err == nil {
		if err := c.WriteJSON(prediction); err != nil {
			log.Printf("⚠️ WebSocket write error: %v", err)
			return
		}
	}

	// Then send updates every 12 seconds
	for range ticker.C {
		prediction, err := h.engine.Predict(market, duration)
		if err != nil {
			log.Printf("⚠️ Prediction error: %v", err)
			continue
		}

		if err := c.WriteJSON(prediction); err != nil {
			log.Printf("⚠️ WebSocket write error: %v", err)
			return
		}

		// Log only high-quality signals to reduce noise
		if prediction.Direction != "NONE" && prediction.Confidence >= 0.72 {
			log.Printf("🎯 HIGH QUALITY SIGNAL: %s %s @ %.1f%% confidence",
				market, prediction.Direction, prediction.Confidence*100)
		}
	}
}

// GetResults handles GET /results/:market
func (h *Handler) GetResults(c *fiber.Ctx) error {
	market := c.Params("market")
	results := h.storage.GetResults(market)
	return c.JSON(results)
}

// GetPerformanceSummary handles GET /performance
func (h *Handler) GetPerformanceSummary(c *fiber.Ctx) error {
	summary := h.tracker.GetPerformanceSummary()
	return c.SendString(summary)
}
