package api

import (
	"log"
	"sort"
	"strconv"
	"time"

	"otc-predictor/internal/predictor"
	"otc-predictor/internal/storage"
	"otc-predictor/internal/tracker"
	"otc-predictor/pkg/types"

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

// MarketOpportunity represents a trading opportunity with quality score
type MarketOpportunity struct {
	Market       string    `json:"market"`
	MarketType   string    `json:"market_type"`
	Direction    string    `json:"direction"`
	Confidence   float64   `json:"confidence"`
	QualityScore float64   `json:"quality_score"`
	CurrentPrice float64   `json:"current_price"`
	Duration     int       `json:"duration"`
	DataPoints   int       `json:"data_points"`
	Reason       string    `json:"reason"`
	WinRate      float64   `json:"win_rate,omitempty"`
	TotalTrades  int       `json:"total_trades,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

// GetBestMarkets analyzes all markets and returns top opportunities
// GET /api/best-markets?duration=60&limit=5&mode=synthetics
func (h *Handler) GetBestMarkets(c *fiber.Ctx) error {
	durationStr := c.Query("duration", "60")
	limitStr := c.Query("limit", "5")
	mode := c.Query("mode", "synthetics") // synthetics, forex, both

	duration, err := strconv.Atoi(durationStr)
	if err != nil || duration < 30 || duration > 3600 {
		duration = 60
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 20 {
		limit = 5
	}

	// Get all active markets
	allMarkets := h.storage.GetActiveMarkets()

	// Filter by mode
	var filteredMarkets []string
	for _, market := range allMarkets {
		marketType := getMarketType(market)

		switch mode {
		case "synthetics":
			if marketType == "volatility" || marketType == "crash_boom" {
				filteredMarkets = append(filteredMarkets, market)
			}
		case "forex":
			if marketType == "forex" {
				filteredMarkets = append(filteredMarkets, market)
			}
		case "both":
			filteredMarkets = append(filteredMarkets, market)
		}
	}

	if len(filteredMarkets) == 0 {
		return c.JSON(fiber.Map{
			"opportunities": []MarketOpportunity{},
			"message":       "No markets available for selected mode",
			"mode":          mode,
		})
	}

	// Analyze all markets
	opportunities := []MarketOpportunity{}

	for _, market := range filteredMarkets {
		pred, err := h.engine.Predict(market, duration)
		if err != nil {
			continue
		}

		// Skip if no clear direction or low confidence
		if pred.Direction == "NONE" || pred.Confidence < 0.65 {
			continue
		}

		// Get historical stats
		stats := h.storage.GetStats(market)

		// Calculate quality score (0-100)
		qualityScore := calculateQualityScore(pred, stats)

		opportunity := MarketOpportunity{
			Market:       pred.Market,
			MarketType:   getMarketType(pred.Market),
			Direction:    pred.Direction,
			Confidence:   pred.Confidence,
			QualityScore: qualityScore,
			CurrentPrice: pred.CurrentPrice,
			Duration:     pred.Duration,
			DataPoints:   pred.DataPoints,
			Reason:       pred.Reason,
			Timestamp:    pred.Timestamp,
		}

		// Add stats if available
		if stats != nil && stats.TotalTrades > 0 {
			opportunity.WinRate = stats.WinRate
			opportunity.TotalTrades = stats.TotalTrades
		}

		opportunities = append(opportunities, opportunity)
	}

	// Sort by quality score (highest first)
	sort.Slice(opportunities, func(i, j int) bool {
		return opportunities[i].QualityScore > opportunities[j].QualityScore
	})

	// Limit results
	if len(opportunities) > limit {
		opportunities = opportunities[:limit]
	}

	return c.JSON(fiber.Map{
		"opportunities":  opportunities,
		"total_analyzed": len(filteredMarkets),
		"total_found":    len(opportunities),
		"mode":           mode,
		"duration":       duration,
		"timestamp":      time.Now(),
	})
}

// calculateQualityScore computes a 0-100 quality score for a prediction
func calculateQualityScore(pred types.Prediction, stats *types.Stats) float64 {
	score := 0.0

	// 1. Confidence weight (40 points max)
	confidenceScore := pred.Confidence * 40.0

	// 2. Historical win rate (30 points max)
	winRateScore := 0.0
	if stats != nil && stats.TotalTrades >= 5 {
		winRateScore = (stats.WinRate / 100.0) * 30.0
	} else {
		// No history = neutral score
		winRateScore = 15.0
	}

	// 3. Data quality (20 points max)
	dataQualityScore := 0.0
	if pred.DataPoints >= 150 {
		dataQualityScore = 20.0
	} else if pred.DataPoints >= 100 {
		dataQualityScore = 15.0
	} else if pred.DataPoints >= 60 {
		dataQualityScore = 10.0
	} else {
		dataQualityScore = 5.0
	}

	// 4. Recent performance (10 points max)
	streakScore := 0.0
	if stats != nil {
		if stats.CurrentStreak > 0 {
			streakScore = 10.0 // On a winning streak
		} else if stats.CurrentStreak < -2 {
			streakScore = 0.0 // On a losing streak
		} else {
			streakScore = 5.0 // Neutral
		}
	} else {
		streakScore = 5.0
	}

	score = confidenceScore + winRateScore + dataQualityScore + streakScore

	// Clamp to 0-100
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	return score
}

// getMarketType determines market category
func getMarketType(market string) string {
	m := market
	if len(m) > 3 {
		m = m[:3]
	}

	switch m {
	case "frx", "AUD", "EUR", "GBP", "USD", "JPY", "CHF", "CAD", "NZD":
		return "forex"
	case "cra", "boo":
		return "crash_boom"
	default:
		return "volatility"
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
