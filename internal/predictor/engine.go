package predictor

import (
	"fmt"
	"sync"
	"time"

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

// getMinimumTicksRequired returns dynamic minimum ticks based on duration
func (e *Engine) getMinimumTicksRequired(duration int) int {
	// For shorter durations, we need less historical data
	switch {
	case duration <= 30:
		return 30 // Just need 30 seconds of data for 30s predictions
	case duration <= 60:
		return 60 // 1 minute of data for 1-minute predictions
	case duration <= 120:
		return 90 // 1.5 minutes for 2-minute predictions
	case duration <= 180:
		return 120 // 2 minutes for 3-minute predictions
	default:
		return 150 // 2.5 minutes for longer predictions
	}
}

// Predict generates a prediction for a market
func (e *Engine) Predict(market string, duration int) (types.Prediction, error) {
	// Check rate limit
	if !e.checkRateLimit(market) {
		return types.Prediction{}, fmt.Errorf("rate limit exceeded for %s", market)
	}

	// Check cache (reuse if < 3 seconds old)
	cacheKey := fmt.Sprintf("%s-%d", market, duration)
	if cached := e.getFromCache(cacheKey); cached != nil {
		if time.Since(cached.Timestamp) < 3*time.Second {
			return cached.Prediction, nil
		}
	}

	// Get ticks
	ticks := e.storage.GetAllTicks(market)

	// Dynamic minimum based on duration
	minRequired := e.getMinimumTicksRequired(duration)

	if len(ticks) < minRequired {
		return types.Prediction{
			Market:     market,
			Direction:  "NONE",
			Confidence: 0,
			Reason: fmt.Sprintf("Collecting data: %d/%d ticks (wait %d more seconds)",
				len(ticks), minRequired, minRequired-len(ticks)),
			Duration:   duration,
			Timestamp:  time.Now(),
			DataPoints: len(ticks),
		}, nil
	}

	// Generate prediction
	prediction := e.strategy.GeneratePrediction(market, ticks, duration)
	prediction.ID = uuid.New().String()

	// Add data quality indicator
	dataQuality := float64(len(ticks)) / float64(e.config.Risk.MinTicksRequired)
	if dataQuality > 1.0 {
		dataQuality = 1.0
	}

	// Adjust confidence based on data quality if less than ideal
	if len(ticks) < e.config.Risk.MinTicksRequired {
		prediction.Confidence *= dataQuality
		if prediction.Reason != "" {
			prediction.Reason += fmt.Sprintf(" (data quality: %.0f%%)", dataQuality*100)
		}
	}

	// Cache it
	e.addToCache(cacheKey, prediction)

	// Track if it's a real prediction
	if prediction.Direction != "NONE" {
		currentPrice := e.storage.GetLatestPrice(market)
		e.tracker.TrackPrediction(prediction, currentPrice)
	}

	return prediction, nil
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

	count := e.requestCounter[market]
	if count >= e.config.Risk.MaxPredictionsPerMinute {
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
		if time.Since(cached.Timestamp) > 10*time.Second {
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
