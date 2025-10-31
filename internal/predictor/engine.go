package predictor

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"otc-predictor/internal/candles"
	"otc-predictor/internal/storage"
	"otc-predictor/internal/strategy"
	"otc-predictor/internal/tracker"
	"otc-predictor/pkg/types"

	"github.com/google/uuid"
)

// Engine is the main prediction engine
type Engine struct {
	storage          *storage.MemoryStorage
	strategy         *strategy.CombinedStrategy
	tracker          *tracker.ResultTracker
	config           types.Config
	cache            map[string]*CachedPrediction
	cacheMu          sync.RWMutex
	requestCounter   map[string]int
	counterMu        sync.Mutex
	lastCounterReset time.Time
}

// CachedPrediction stores a recent prediction
type CachedPrediction struct {
	Prediction types.Prediction
	Timestamp  time.Time
}

// NewEngine creates a new prediction engine
func NewEngine(storage *storage.MemoryStorage, config types.Config, tracker *tracker.ResultTracker) *Engine {
	return &Engine{
		storage:          storage,
		strategy:         strategy.NewCombinedStrategy(config.Strategy),
		tracker:          tracker,
		config:           config,
		cache:            make(map[string]*CachedPrediction),
		requestCounter:   make(map[string]int),
		lastCounterReset: time.Now(),
	}
}

// Predict generates a timeframe-aware prediction
func (e *Engine) Predict(market string, duration int) (types.Prediction, error) {
	// Check rate limit
	if !e.checkRateLimit(market) {
		return types.Prediction{}, fmt.Errorf("rate limit exceeded for %s", market)
	}

	// Check cache (reuse if < duration-based cache time)
	cacheKey := fmt.Sprintf("%s-%d", market, duration)
	cacheTimeout := e.getCacheTimeout(duration)

	if cached := e.getFromCache(cacheKey); cached != nil {
		if time.Since(cached.Timestamp) < cacheTimeout {
			return cached.Prediction, nil
		}
	}

	// Get market type
	marketType := getMarketTypeHelper(market)

	// Get timeframe configuration
	tfConfig := candles.GetTimeframeConfig(duration, marketType)

	// Get all ticks
	ticks := e.storage.GetAllTicks(market)

	// Calculate minimum ticks required
	minRequired := candles.GetMinimumTicksRequired(tfConfig, marketType)

	if len(ticks) < minRequired {
		ticksPerMin := e.estimateTickRate(marketType)
		minutesNeeded := (minRequired - len(ticks)) / ticksPerMin
		if minutesNeeded < 1 {
			minutesNeeded = 1
		}

		return types.Prediction{
			Market:     market,
			MarketType: marketType,
			Direction:  "NONE",
			Confidence: 0,
			Reason: fmt.Sprintf("Collecting data: %d/%d ticks (~%d min remaining)",
				len(ticks), minRequired, minutesNeeded),
			Duration:   duration,
			Timestamp:  time.Now(),
			DataPoints: len(ticks),
		}, nil
	}

	// Convert ticks to candles
	candleData := candles.TicksToCandles(ticks, tfConfig.CandlePeriod)

	// Validate candle quality
	valid, reason := candles.ValidateCandles(candleData, tfConfig.MinCandles)
	if !valid {
		return types.Prediction{
			Market:     market,
			MarketType: marketType,
			Direction:  "NONE",
			Confidence: 0,
			Reason:     fmt.Sprintf("Data quality issue: %s (candles: %d/%d)", reason, len(candleData), tfConfig.MinCandles),
			Duration:   duration,
			Timestamp:  time.Now(),
			DataPoints: len(ticks),
		}, nil
	}

	// Convert candles back to ticks for indicator calculation
	candleTicks := candles.CandlesToTicks(candleData)

	// Generate prediction with timeframe-aware config
	prediction := e.strategy.GeneratePrediction(
		market,
		candleTicks,
		duration,
	)
	prediction.ID = uuid.New().String()
	prediction.MarketType = marketType

	// Quality boost for good data
	if len(candleData) >= tfConfig.MinCandles*2 {
		prediction.Confidence *= 1.05 // 5% boost for abundant data
		if prediction.Confidence > 0.95 {
			prediction.Confidence = 0.95
		}
	}

	// Cache it with longer timeout for longer durations
	e.addToCache(cacheKey, prediction)

	// Track if it's a real prediction
	if prediction.Direction != "NONE" {
		currentPrice := e.storage.GetLatestPrice(market)
		e.tracker.TrackPrediction(prediction, currentPrice)
	}

	return prediction, nil
}

// getCacheTimeout returns appropriate cache timeout based on duration
func (e *Engine) getCacheTimeout(duration int) time.Duration {
	switch {
	case duration <= 60:
		return 5 * time.Second // Short trades: refresh often
	case duration <= 300:
		return 10 * time.Second // Medium trades
	case duration <= 900:
		return 20 * time.Second // 15-min trades
	default:
		return 30 * time.Second // Long trades
	}
}

// estimateTickRate estimates ticks per minute for a market type
func (e *Engine) estimateTickRate(marketType string) int {
	switch marketType {
	case "forex":
		return 2 // ~2 ticks/min average
	case "volatility":
		return 60 // ~1 tick/sec
	case "crash_boom":
		return 30 // ~0.5 tick/sec
	default:
		return 10
	}
}

// PredictAll generates predictions for all active markets
func (e *Engine) PredictAll(duration int) map[string]types.Prediction {
	markets := e.storage.GetActiveMarkets()
	predictions := make(map[string]types.Prediction)

	for _, market := range markets {
		pred, err := e.Predict(market, duration)
		if err == nil {
			predictions[market] = pred
		}
	}

	return predictions
}

// checkRateLimit checks if request is within rate limit
func (e *Engine) checkRateLimit(market string) bool {
	e.counterMu.Lock()
	defer e.counterMu.Unlock()

	// Reset counter every minute
	if time.Since(e.lastCounterReset) > time.Minute {
		e.requestCounter = make(map[string]int)
		e.lastCounterReset = time.Now()
	}

	maxPerMinute := 10
	if e.config.Risk.MaxPredictionsPerMinute > 0 {
		maxPerMinute = e.config.Risk.MaxPredictionsPerMinute
	}

	count := e.requestCounter[market]
	if count >= maxPerMinute {
		return false
	}

	e.requestCounter[market]++
	return true
}

// getFromCache retrieves cached prediction
func (e *Engine) getFromCache(key string) *CachedPrediction {
	e.cacheMu.RLock()
	defer e.cacheMu.RUnlock()

	return e.cache[key]
}

// addToCache adds prediction to cache
func (e *Engine) addToCache(key string, prediction types.Prediction) {
	e.cacheMu.Lock()
	defer e.cacheMu.Unlock()

	e.cache[key] = &CachedPrediction{
		Prediction: prediction,
		Timestamp:  time.Now(),
	}
}

// CleanupCache removes old cached predictions
func (e *Engine) CleanupCache() {
	e.cacheMu.Lock()
	defer e.cacheMu.Unlock()

	for key, cached := range e.cache {
		if time.Since(cached.Timestamp) > 60*time.Second {
			delete(e.cache, key)
		}
	}
}

// GetStats returns statistics for a market
func (e *Engine) GetStats(market string) *types.Stats {
	return e.storage.GetStats(market)
}

// GetAllStats returns statistics for all markets
func (e *Engine) GetAllStats() map[string]*types.Stats {
	return e.storage.GetAllStats()
}

// getMarketTypeHelper determines market type
func getMarketTypeHelper(market string) string {
	m := strings.ToLower(market)

	// Check for forex
	if strings.Contains(m, "frx") || strings.Contains(m, "aud") ||
		strings.Contains(m, "eur") || strings.Contains(m, "gbp") ||
		strings.Contains(m, "usd") || strings.Contains(m, "jpy") ||
		strings.Contains(m, "chf") || strings.Contains(m, "cad") {
		return "forex"
	}

	// Check for crash/boom
	if strings.Contains(m, "crash") || strings.Contains(m, "boom") {
		return "crash_boom"
	}

	// Default to volatility
	return "volatility"
}
