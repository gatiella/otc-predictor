package storage

import (
	"otc-predictor/pkg/types"
	"sync"
	"time"
)

// MemoryStorage stores all data in memory
type MemoryStorage struct {
	markets     map[string]*types.MarketData
	predictions map[string][]types.Prediction
	results     map[string][]types.TradeResult
	pending     map[string]*types.PendingPrediction
	stats       map[string]*types.Stats
	mu          sync.RWMutex
	maxTicks    int
}

// NewMemoryStorage creates a new memory storage
func NewMemoryStorage(maxTicks int) *MemoryStorage {
	return &MemoryStorage{
		markets:     make(map[string]*types.MarketData),
		predictions: make(map[string][]types.Prediction),
		results:     make(map[string][]types.TradeResult),
		pending:     make(map[string]*types.PendingPrediction),
		stats:       make(map[string]*types.Stats),
		maxTicks:    maxTicks,
	}
}

// AddTick adds a new tick to market data
func (s *MemoryStorage) AddTick(market string, tick types.Tick) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.markets[market] == nil {
		s.markets[market] = &types.MarketData{
			Market:   market,
			Ticks:    []types.Tick{},
			IsActive: true,
		}
	}

	s.markets[market].Ticks = append(s.markets[market].Ticks, tick)
	s.markets[market].LastUpdate = tick.Timestamp

	// Keep only last N ticks
	if len(s.markets[market].Ticks) > s.maxTicks {
		s.markets[market].Ticks = s.markets[market].Ticks[len(s.markets[market].Ticks)-s.maxTicks:]
	}
}

// GetTicks returns last N ticks for a market
func (s *MemoryStorage) GetTicks(market string, n int) []types.Tick {
	s.mu.RLock()
	defer s.mu.RUnlock()

	marketData, exists := s.markets[market]
	if !exists || len(marketData.Ticks) == 0 {
		return []types.Tick{}
	}

	ticks := marketData.Ticks
	if len(ticks) > n {
		return ticks[len(ticks)-n:]
	}

	return ticks
}

// GetAllTicks returns all ticks for a market
func (s *MemoryStorage) GetAllTicks(market string) []types.Tick {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if marketData, exists := s.markets[market]; exists {
		return marketData.Ticks
	}

	return []types.Tick{}
}

// GetLatestPrice returns the most recent price
func (s *MemoryStorage) GetLatestPrice(market string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if marketData, exists := s.markets[market]; exists && len(marketData.Ticks) > 0 {
		return marketData.Ticks[len(marketData.Ticks)-1].Price
	}

	return 0
}

// StorePrediction stores a prediction
func (s *MemoryStorage) StorePrediction(pred types.Prediction) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.predictions[pred.Market] = append(s.predictions[pred.Market], pred)
}

// StorePendingPrediction stores a pending prediction
func (s *MemoryStorage) StorePendingPrediction(pending *types.PendingPrediction) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pending[pending.ID] = pending
}

// GetPendingPrediction retrieves a pending prediction
func (s *MemoryStorage) GetPendingPrediction(id string) (*types.PendingPrediction, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pending, exists := s.pending[id]
	return pending, exists
}

// RemovePendingPrediction removes a pending prediction
func (s *MemoryStorage) RemovePendingPrediction(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.pending, id)
}

// StoreResult stores a trade result
func (s *MemoryStorage) StoreResult(result types.TradeResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.results[result.Market] = append(s.results[result.Market], result)
}

// GetResults returns all results for a market
func (s *MemoryStorage) GetResults(market string) []types.TradeResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if results, exists := s.results[market]; exists {
		return results
	}

	return []types.TradeResult{}
}

// GetAllResults returns results for all markets
func (s *MemoryStorage) GetAllResults() map[string][]types.TradeResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	allResults := make(map[string][]types.TradeResult)
	for market, results := range s.results {
		allResults[market] = append([]types.TradeResult{}, results...)
	}

	return allResults
}

// UpdateStats updates statistics for a market
func (s *MemoryStorage) UpdateStats(market string, stats types.Stats) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stats[market] = &stats
}

// GetStats returns statistics for a market
func (s *MemoryStorage) GetStats(market string) *types.Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if stats, exists := s.stats[market]; exists {
		return stats
	}

	return &types.Stats{
		Market:      market,
		LastUpdated: time.Now(),
	}
}

// GetAllStats returns statistics for all markets
func (s *MemoryStorage) GetAllStats() map[string]*types.Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	allStats := make(map[string]*types.Stats)
	for market, stats := range s.stats {
		statsCopy := *stats
		allStats[market] = &statsCopy
	}

	return allStats
}

// GetActiveMarkets returns list of active markets
func (s *MemoryStorage) GetActiveMarkets() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	markets := []string{}
	for market, data := range s.markets {
		if data.IsActive && len(data.Ticks) > 0 {
			markets = append(markets, market)
		}
	}

	return markets
}

// GetTickCount returns number of ticks for a market
func (s *MemoryStorage) GetTickCount(market string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if marketData, exists := s.markets[market]; exists {
		return len(marketData.Ticks)
	}

	return 0
}

// Cleanup removes old data
func (s *MemoryStorage) Cleanup(keepHours int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-time.Duration(keepHours) * time.Hour)

	// Clean old predictions
	for market := range s.predictions {
		filtered := []types.Prediction{}
		for _, pred := range s.predictions[market] {
			if pred.Timestamp.After(cutoff) {
				filtered = append(filtered, pred)
			}
		}
		s.predictions[market] = filtered
	}
}
