package api

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"sync"
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

	// 🔧 PERFORMANCE: Cache for batch analysis
	analysisCache map[string]*CachedAnalysis
	cacheMu       sync.RWMutex
	cacheExpiry   time.Duration
	lastCleanup   time.Time
}

// 🔧 PERFORMANCE: Cache entry for market analysis
type CachedAnalysis struct {
	Market    string
	Analysis  MarketOpportunity
	Timestamp time.Time
	IsActive  bool
}

// 🔧 PERFORMANCE: Batch result cache
type BatchAnalysisCache struct {
	Results      map[string]MarketOpportunity
	Timestamp    time.Time
	Duration     int
	Mode         string
	TotalMarkets int
}

// NewHandler creates a new API handler
func NewHandler(engine *predictor.Engine, storage *storage.MemoryStorage, tracker *tracker.ResultTracker) *Handler {
	return &Handler{
		engine:        engine,
		storage:       storage,
		tracker:       tracker,
		analysisCache: make(map[string]*CachedAnalysis),
		cacheExpiry:   30 * time.Second, // Cache for 30 seconds
		lastCleanup:   time.Now(),
	}
}

// 🔧 PERFORMANCE: Cleanup expired cache entries
func (h *Handler) cleanupCache() {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()

	now := time.Now()
	for key, cached := range h.analysisCache {
		if now.Sub(cached.Timestamp) > h.cacheExpiry {
			delete(h.analysisCache, key)
		}
	}
	h.lastCleanup = now
}

// 🔧 PERFORMANCE: Get cached analysis or compute new one
func (h *Handler) getCachedAnalysis(market string, duration int) *MarketOpportunity {
	h.cleanupCache()

	h.cacheMu.RLock()
	defer h.cacheMu.RUnlock()

	cacheKey := fmt.Sprintf("%s-%d", market, duration)
	if cached := h.analysisCache[cacheKey]; cached != nil && cached.IsActive {
		if time.Since(cached.Timestamp) < h.cacheExpiry {
			return &cached.Analysis
		}
	}
	return nil
}

// 🔧 PERFORMANCE: Store analysis in cache
func (h *Handler) storeAnalysis(market string, duration int, analysis MarketOpportunity) {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()

	cacheKey := fmt.Sprintf("%s-%d", market, duration)
	h.analysisCache[cacheKey] = &CachedAnalysis{
		Market:    market,
		Analysis:  analysis,
		Timestamp: time.Now(),
		IsActive:  true,
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

// 🔧 PERFORMANCE: Fast batch analysis with smart caching
// This solves the "25 forex pairs taking too long" problem
func (h *Handler) GetBestMarkets(c *fiber.Ctx) error {
	durationStr := c.Query("duration", "60")
	limitStr := c.Query("limit", "5")
	mode := c.Query("mode", "synthetics")

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

	// 🔧 PERFORMANCE: Smart processing order - synthetics first (faster)
	forexMarkets := []string{}
	synthMarkets := []string{}

	for _, market := range filteredMarkets {
		if getMarketType(market) == "forex" {
			forexMarkets = append(forexMarkets, market)
		} else {
			synthMarkets = append(synthMarkets, market)
		}
	}

	// Process synthetics first (they get data faster), then forex
	filteredMarkets = append(synthMarkets, forexMarkets...)

	// 🔧 PERFORMANCE: Use caching + smart limits
	opportunities := []MarketOpportunity{}
	processed := 0
	cacheHits := 0

	for _, market := range filteredMarkets {
		var prediction types.Prediction

		// Try cache first for faster response
		if cached := h.getCachedAnalysis(market, duration); cached != nil {
			prediction = types.Prediction{
				Market:       cached.Market,
				Direction:    cached.Direction,
				Confidence:   cached.Confidence,
				CurrentPrice: cached.CurrentPrice,
				Duration:     cached.Duration,
				DataPoints:   cached.DataPoints,
				Reason:       cached.Reason,
				Timestamp:    cached.Timestamp,
			}
			cacheHits++
		} else {
			// Only make API call if not cached
			prediction, err = h.engine.Predict(market, duration)
			if err != nil {
				continue
			}
		}

		// Skip if no clear direction or confidence below 58%
		if prediction.Direction == "NONE" || prediction.Confidence < 0.58 {
			continue
		}

		// Get historical stats (cached operation)
		stats := h.storage.GetStats(market)

		// Calculate quality score
		qualityScore := calculateQualityScore(prediction, stats)

		opportunity := MarketOpportunity{
			Market:       prediction.Market,
			MarketType:   getMarketType(prediction.Market),
			Direction:    prediction.Direction,
			Confidence:   prediction.Confidence,
			QualityScore: qualityScore,
			CurrentPrice: prediction.CurrentPrice,
			Duration:     prediction.Duration,
			DataPoints:   prediction.DataPoints,
			Reason:       prediction.Reason,
			Timestamp:    prediction.Timestamp,
		}

		// Add stats if available
		if stats != nil && stats.TotalTrades > 0 {
			opportunity.WinRate = stats.WinRate
			opportunity.TotalTrades = stats.TotalTrades
		}

		// Cache this analysis for future requests
		h.storeAnalysis(market, duration, opportunity)

		opportunities = append(opportunities, opportunity)
		processed++

		// 🔧 PERFORMANCE: Early exit when we have enough good opportunities
		if processed >= limit*3 && len(opportunities) >= limit {
			break
		}
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
		"cache_used":     fmt.Sprintf("%.1f%%", float64(cacheHits)/float64(processed)*100),
	})
}

// calculateQualityScore computes a 0-100 quality score for a prediction
// 🔧 FIXED: Adjusted for new 58% confidence threshold
func calculateQualityScore(pred types.Prediction, stats *types.Stats) float64 {
	score := 0.0

	// 1. Confidence weight (40 points max)
	// ✅ FIXED: Normalized for 58% minimum (was 65%)
	// At 58% confidence = 0 bonus points, at 82% = full 40 points
	normalizedConf := (pred.Confidence - 0.58) / (0.82 - 0.58)
	if normalizedConf < 0 {
		normalizedConf = 0
	}
	confidenceScore := normalizedConf * 40.0

	// 2. Historical win rate (30 points max)
	winRateScore := 0.0
	if stats != nil && stats.TotalTrades >= 5 {
		// Scale: 50% = 0 points, 70% = 30 points
		winRateScore = ((stats.WinRate - 50.0) / 20.0) * 30.0
		if winRateScore < 0 {
			winRateScore = 0
		}
		if winRateScore > 30 {
			winRateScore = 30
		}
	} else {
		// No history = neutral score (15 points)
		winRateScore = 15.0
	}

	// 3. Data quality (20 points max)
	// ✅ FIXED: Adjusted thresholds for new minimum requirements
	dataQualityScore := 0.0
	marketType := getMarketType(pred.Market)

	if marketType == "forex" {
		// Forex: 60 min, 80 good, 120+ excellent
		if pred.DataPoints >= 120 {
			dataQualityScore = 20.0
		} else if pred.DataPoints >= 80 {
			dataQualityScore = 15.0
		} else if pred.DataPoints >= 60 {
			dataQualityScore = 10.0
		} else {
			dataQualityScore = 5.0
		}
	} else {
		// Synthetics: 40 min, 60 good, 100+ excellent
		if pred.DataPoints >= 100 {
			dataQualityScore = 20.0
		} else if pred.DataPoints >= 60 {
			dataQualityScore = 15.0
		} else if pred.DataPoints >= 40 {
			dataQualityScore = 10.0
		} else {
			dataQualityScore = 5.0
		}
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
	if err != nil || duration < 30 || duration > 3600 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid duration (must be between 30-3600 seconds)",
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
	if err != nil || duration < 30 || duration > 3600 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid duration (must be between 30-3600 seconds)",
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
// ✅ OPTIMIZED: Updates every 10 seconds (6 requests/minute - within new limit)
func (h *Handler) WebSocketHandler(c *websocket.Conn) {
	market := c.Params("market")
	durationStr := c.Query("duration", "60")

	duration, err := strconv.Atoi(durationStr)
	if err != nil || duration < 30 || duration > 3600 {
		duration = 60
	}

	log.Printf("📡 WebSocket connected: %s (duration: %ds, optimized mode)", market, duration)

	defer func() {
		c.Close()
		log.Printf("📡 WebSocket disconnected: %s", market)
	}()

	// ✅ FIXED: Send predictions every 10 seconds (6/min - within new 10/min limit)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Send initial prediction immediately
	prediction, err := h.engine.Predict(market, duration)
	if err == nil {
		if err := c.WriteJSON(prediction); err != nil {
			log.Printf("⚠️ WebSocket write error: %v", err)
			return
		}
	}

	// Then send updates every 10 seconds
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

		// ✅ Log high-quality signals (adjusted threshold: 65% → 58%)
		if prediction.Direction != "NONE" && prediction.Confidence >= 0.65 {
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
