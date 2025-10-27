package tracker

import (
	"fmt"
	"log"
	"otc-predictor/internal/storage"
	"otc-predictor/pkg/types"
	"strings"
	"time"
)

// ResultTracker tracks prediction outcomes
type ResultTracker struct {
	storage *storage.MemoryStorage
}

// NewResultTracker creates a new result tracker
func NewResultTracker(storage *storage.MemoryStorage) *ResultTracker {
	return &ResultTracker{
		storage: storage,
	}
}

// TrackPrediction starts tracking a prediction
func (t *ResultTracker) TrackPrediction(pred types.Prediction, currentPrice float64) {
	if pred.Direction == "NONE" {
		return
	}

	pending := &types.PendingPrediction{
		ID:         pred.ID,
		Market:     pred.Market,
		Direction:  pred.Direction,
		EntryPrice: currentPrice,
		EntryTime:  pred.Timestamp,
		Duration:   pred.Duration,
		Confidence: pred.Confidence,
		ExpiryTime: pred.Timestamp.Add(time.Duration(pred.Duration) * time.Second),
	}

	t.storage.StorePendingPrediction(pending)

	// Schedule result check
	go t.checkResultLater(pending)
}

// checkResultLater waits for duration then checks result
func (t *ResultTracker) checkResultLater(pending *types.PendingPrediction) {
	time.Sleep(time.Duration(pending.Duration) * time.Second)

	// Get current price
	currentPrice := t.storage.GetLatestPrice(pending.Market)

	if currentPrice == 0 {
		log.Printf("Warning: No price available for %s, skipping result tracking", pending.Market)
		t.storage.RemovePendingPrediction(pending.ID)
		return
	}

	// Determine if won
	won := false
	priceChange := currentPrice - pending.EntryPrice

	if pending.Direction == "UP" && currentPrice > pending.EntryPrice {
		won = true
	} else if pending.Direction == "DOWN" && currentPrice < pending.EntryPrice {
		won = true
	}

	// Calculate P/L (assuming $10 stake, 85% payout)
	profitLoss := -10.0
	if won {
		profitLoss = 8.5 // 85% profit
	}

	// Create result
	result := types.TradeResult{
		PredictionID: pending.ID,
		Market:       pending.Market,
		Direction:    pending.Direction,
		EntryPrice:   pending.EntryPrice,
		ExitPrice:    currentPrice,
		EntryTime:    pending.EntryTime,
		ExitTime:     time.Now(),
		Duration:     pending.Duration,
		Confidence:   pending.Confidence,
		Won:          won,
		ProfitLoss:   profitLoss,
		PriceChange:  priceChange,
	}

	// Store result
	t.storage.StoreResult(result)
	t.storage.RemovePendingPrediction(pending.ID)

	// Log result
	status := "❌ LOSS"
	if won {
		status = "✅ WIN"
	}

	log.Printf("%s | %s %s | Entry: %.5f → Exit: %.5f | Conf: %.1f%% | P/L: $%.2f",
		status, pending.Market, pending.Direction,
		pending.EntryPrice, currentPrice,
		pending.Confidence*100, profitLoss)

	// Update statistics
	t.UpdateStats(pending.Market)
}

// UpdateStats calculates and updates statistics
func (t *ResultTracker) UpdateStats(market string) {
	results := t.storage.GetResults(market)

	if len(results) == 0 {
		return
	}

	wins := 0
	totalPL := 0.0
	totalConfidence := 0.0
	currentStreak := 0
	bestStreak := 0
	tempStreak := 0

	for i, result := range results {
		if result.Won {
			wins++
			tempStreak++
			if tempStreak > bestStreak {
				bestStreak = tempStreak
			}
		} else {
			tempStreak = 0
		}

		totalPL += result.ProfitLoss
		totalConfidence += result.Confidence

		// Current streak (from end)
		if i == len(results)-1 {
			currentStreak = tempStreak
		}
	}

	winRate := float64(wins) / float64(len(results)) * 100
	avgConfidence := totalConfidence / float64(len(results))

	// Get recent trades
	recentCount := 20
	if len(results) < recentCount {
		recentCount = len(results)
	}
	recentTrades := results[len(results)-recentCount:]

	stats := types.Stats{
		Market:          market,
		TotalTrades:     len(results),
		Wins:            wins,
		Losses:          len(results) - wins,
		WinRate:         winRate,
		TotalProfitLoss: totalPL,
		AvgConfidence:   avgConfidence,
		BestStreak:      bestStreak,
		CurrentStreak:   currentStreak,
		LastUpdated:     time.Now(),
		RecentTrades:    recentTrades,
	}

	t.storage.UpdateStats(market, stats)
}

// CalculateAllStats updates stats for all markets
func (t *ResultTracker) CalculateAllStats() {
	allResults := t.storage.GetAllResults()

	for market := range allResults {
		t.UpdateStats(market)
	}
}

// GetPerformanceSummary returns overall performance
func (t *ResultTracker) GetPerformanceSummary() string {
	allStats := t.storage.GetAllStats()

	if len(allStats) == 0 {
		return "No trades yet"
	}

	totalTrades := 0
	totalWins := 0
	totalPL := 0.0

	summary := "\n" + strings.Repeat("=", 60) + "\n"
	summary += "📊 PERFORMANCE SUMMARY\n"
	summary += strings.Repeat("=", 60) + "\n\n"

	for market, stats := range allStats {
		if stats.TotalTrades == 0 {
			continue
		}

		totalTrades += stats.TotalTrades
		totalWins += stats.Wins
		totalPL += stats.TotalProfitLoss

		summary += fmt.Sprintf("%-20s | Trades: %3d | Win Rate: %5.1f%% | P/L: $%7.2f | Streak: %d\n",
			market, stats.TotalTrades, stats.WinRate, stats.TotalProfitLoss, stats.CurrentStreak)
	}

	summary += "\n" + strings.Repeat("-", 60) + "\n"

	overallWinRate := 0.0
	if totalTrades > 0 {
		overallWinRate = float64(totalWins) / float64(totalTrades) * 100
	}

	summary += fmt.Sprintf("OVERALL              | Trades: %3d | Win Rate: %5.1f%% | P/L: $%7.2f\n",
		totalTrades, overallWinRate, totalPL)
	summary += strings.Repeat("=", 60) + "\n"

	return summary
}
