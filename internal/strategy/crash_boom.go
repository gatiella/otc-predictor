package strategy

import (
	"math"
	"otc-predictor/pkg/types"
	"strings"
)

// CrashBoomStrategy for Crash/Boom indices
type CrashBoomStrategy struct {
	config types.StrategyConfig
}

// NewCrashBoomStrategy creates a new crash/boom strategy
func NewCrashBoomStrategy(config types.StrategyConfig) *CrashBoomStrategy {
	return &CrashBoomStrategy{
		config: config,
	}
}

// Analyze generates signals for crash/boom indices
func (s *CrashBoomStrategy) Analyze(ticks []types.Tick, inds types.Indicators, market string) []types.StrategySignal {
	signals := []types.StrategySignal{}

	if len(ticks) < 60 {
		return signals
	}

	isCrash := strings.Contains(strings.ToLower(market), "crash")
	isBoom := strings.Contains(strings.ToLower(market), "boom")

	// Get spike statistics
	spikeStats := s.analyzeSpikePattern(ticks)

	// Strategy 1: Spike prediction (HIGHEST CONFIDENCE)
	spikeSignal := s.spikeDetectionSignal(ticks, spikeStats, isCrash, isBoom)
	if spikeSignal.Direction != "NONE" {
		signals = append(signals, spikeSignal)
	}

	// Strategy 2: Between-spike trend (MEDIUM CONFIDENCE)
	trendSignal := s.betweenSpikeTrendSignal(ticks, inds, spikeStats, isCrash, isBoom)
	if trendSignal.Direction != "NONE" {
		signals = append(signals, trendSignal)
	}

	// Strategy 3: Pre-spike volatility indicator (NEW)
	preSpikeSignal := s.preSpikeVolatilitySignal(ticks, inds, spikeStats, isCrash, isBoom)
	if preSpikeSignal.Direction != "NONE" {
		signals = append(signals, preSpikeSignal)
	}

	// Strategy 4: Post-spike recovery (NEW)
	recoverySignal := s.postSpikeRecoverySignal(ticks, spikeStats, isCrash, isBoom)
	if recoverySignal.Direction != "NONE" {
		signals = append(signals, recoverySignal)
	}

	return signals
}

// SpikeStats holds spike pattern information
type SpikeStats struct {
	LastSpikeIdx    int
	AvgInterval     float64
	TicksSinceSpike int
	SpikeDue        bool
	RecentSpikes    []int
	Confidence      float64
}

// analyzeSpikePattern analyzes the spike pattern
func (s *CrashBoomStrategy) analyzeSpikePattern(ticks []types.Tick) SpikeStats {
	stats := SpikeStats{
		LastSpikeIdx: -1,
		RecentSpikes: []int{},
	}

	if len(ticks) < 20 {
		return stats
	}

	// Detect spikes with improved algorithm
	for i := 10; i < len(ticks)-10; i++ {
		if s.isSpikePoint(ticks, i) {
			stats.RecentSpikes = append(stats.RecentSpikes, i)
		}
	}

	if len(stats.RecentSpikes) == 0 {
		return stats
	}

	stats.LastSpikeIdx = stats.RecentSpikes[len(stats.RecentSpikes)-1]
	stats.TicksSinceSpike = len(ticks) - stats.LastSpikeIdx - 1

	// Calculate average interval
	if len(stats.RecentSpikes) >= 2 {
		totalInterval := 0.0
		for i := 1; i < len(stats.RecentSpikes); i++ {
			totalInterval += float64(stats.RecentSpikes[i] - stats.RecentSpikes[i-1])
		}
		stats.AvgInterval = totalInterval / float64(len(stats.RecentSpikes)-1)

		// Check if spike is due (within 85-110% of average interval)
		progress := float64(stats.TicksSinceSpike) / stats.AvgInterval
		if progress >= 0.85 && progress <= 1.10 {
			stats.SpikeDue = true
			// Confidence increases as we approach the expected spike time
			stats.Confidence = math.Min(0.75, 0.55+(progress-0.85)*0.8)
		}
	} else {
		// Only one spike found, use default interval
		stats.AvgInterval = 70.0
		if stats.TicksSinceSpike >= 60 {
			stats.SpikeDue = true
			stats.Confidence = 0.62
		}
	}

	return stats
}

// isSpikePoint detects if a point is a spike
func (s *CrashBoomStrategy) isSpikePoint(ticks []types.Tick, idx int) bool {
	if idx < 5 || idx >= len(ticks)-5 {
		return false
	}

	currentPrice := ticks[idx].Price

	// Calculate moving average before and after
	beforeSum := 0.0
	afterSum := 0.0

	for i := idx - 5; i < idx; i++ {
		beforeSum += ticks[i].Price
	}
	beforeAvg := beforeSum / 5.0

	for i := idx + 1; i <= idx+5; i++ {
		afterSum += ticks[i].Price
	}
	afterAvg := afterSum / 5.0

	// Check for sudden spike (>3% deviation)
	changeFromBefore := math.Abs((currentPrice - beforeAvg) / beforeAvg)
	changeToAfter := math.Abs((afterAvg - currentPrice) / currentPrice)

	// Also check immediate change (spike should be sharp)
	immediateChange := math.Abs((currentPrice - ticks[idx-1].Price) / ticks[idx-1].Price)

	return (changeFromBefore > 0.025 || changeToAfter > 0.025) && immediateChange > 0.015
}

// spikeDetectionSignal predicts when spike is imminent
func (s *CrashBoomStrategy) spikeDetectionSignal(ticks []types.Tick, stats SpikeStats, isCrash, isBoom bool) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "SpikeDetection",
		Weight: 0.50,
	}

	if !stats.SpikeDue {
		signal.Direction = "NONE"
		return signal
	}

	// Check for pre-spike indicators
	recentVolatility := s.calculateRecentVolatility(ticks, 10)
	baseline := s.calculateRecentVolatility(ticks, 30)

	// Volatility increasing (common before spikes)
	volatilityIncreasing := recentVolatility > baseline*1.15

	confidence := stats.Confidence

	if volatilityIncreasing {
		confidence += 0.06
	}

	// Price behavior near expected spike time
	if stats.TicksSinceSpike >= int(stats.AvgInterval*0.95) {
		confidence += 0.04 // Very close to spike time
	}

	if isCrash {
		signal.Direction = "DOWN"
		signal.Confidence = math.Min(0.78, confidence)
		signal.Reason = "Crash spike imminent (pattern-based)"
		return signal
	}

	if isBoom {
		signal.Direction = "UP"
		signal.Confidence = math.Min(0.78, confidence)
		signal.Reason = "Boom spike imminent (pattern-based)"
		return signal
	}

	signal.Direction = "NONE"
	return signal
}

// betweenSpikeTrendSignal for the drift period between spikes
func (s *CrashBoomStrategy) betweenSpikeTrendSignal(ticks []types.Tick, inds types.Indicators, stats SpikeStats, isCrash, isBoom bool) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "BetweenSpikeTrend",
		Weight: 0.30,
	}

	if stats.LastSpikeIdx == -1 || stats.AvgInterval == 0 {
		signal.Direction = "NONE"
		return signal
	}

	progress := float64(stats.TicksSinceSpike) / stats.AvgInterval

	// Early to mid period between spikes (20%-70% of interval)
	if progress < 0.20 || progress > 0.75 {
		signal.Direction = "NONE"
		return signal
	}

	// Crash indices: Rise between crashes (gravity recovery)
	if isCrash && inds.RSI < 65 && inds.Momentum >= -0.001 {
		confidence := 0.66

		// Stronger confidence in mid-range
		if progress >= 0.35 && progress <= 0.60 {
			confidence = 0.70
		}

		// Trend confirmation
		if inds.EMA9 > inds.EMA21 {
			confidence += 0.04
		}

		signal.Direction = "UP"
		signal.Confidence = confidence
		signal.Reason = "Between-crash upward drift"
		return signal
	}

	// Boom indices: Fall between booms (mean reversion)
	if isBoom && inds.RSI > 35 && inds.Momentum <= 0.001 {
		confidence := 0.66

		// Stronger confidence in mid-range
		if progress >= 0.35 && progress <= 0.60 {
			confidence = 0.70
		}

		// Trend confirmation
		if inds.EMA9 < inds.EMA21 {
			confidence += 0.04
		}

		signal.Direction = "DOWN"
		signal.Confidence = confidence
		signal.Reason = "Between-boom downward drift"
		return signal
	}

	signal.Direction = "NONE"
	return signal
}

// preSpikeVolatilitySignal - NEW: Detects volatility changes before spike
func (s *CrashBoomStrategy) preSpikeVolatilitySignal(ticks []types.Tick, inds types.Indicators, stats SpikeStats, isCrash, isBoom bool) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "PreSpikeVolatility",
		Weight: 0.25,
	}

	if stats.LastSpikeIdx == -1 || stats.AvgInterval == 0 {
		signal.Direction = "NONE"
		return signal
	}

	progress := float64(stats.TicksSinceSpike) / stats.AvgInterval

	// Only active in late stage (80-100% of interval)
	if progress < 0.80 || progress > 1.05 {
		signal.Direction = "NONE"
		return signal
	}

	// Check for elevated volatility
	recentVol := s.calculateRecentVolatility(ticks, 15)
	baselineVol := s.calculateRecentVolatility(ticks, 50)

	// Volatility spike detected
	if recentVol > baselineVol*1.25 {
		confidence := 0.68

		// Very high volatility
		if recentVol > baselineVol*1.5 {
			confidence = 0.72
		}

		if isCrash {
			signal.Direction = "DOWN"
			signal.Confidence = confidence
			signal.Reason = "Pre-crash volatility surge"
			return signal
		}

		if isBoom {
			signal.Direction = "UP"
			signal.Confidence = confidence
			signal.Reason = "Pre-boom volatility surge"
			return signal
		}
	}

	signal.Direction = "NONE"
	return signal
}

// postSpikeRecoverySignal - NEW: Trade the recovery after spike
func (s *CrashBoomStrategy) postSpikeRecoverySignal(ticks []types.Tick, stats SpikeStats, isCrash, isBoom bool) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "PostSpikeRecovery",
		Weight: 0.20,
	}

	if stats.LastSpikeIdx == -1 {
		signal.Direction = "NONE"
		return signal
	}

	// Only valid very shortly after spike (1-10 ticks)
	if stats.TicksSinceSpike < 1 || stats.TicksSinceSpike > 10 {
		signal.Direction = "NONE"
		return signal
	}

	// Verify it was a real spike
	if stats.LastSpikeIdx >= len(ticks)-1 {
		signal.Direction = "NONE"
		return signal
	}

	spikePrice := ticks[stats.LastSpikeIdx].Price
	currentPrice := ticks[len(ticks)-1].Price

	// After crash spike, price should recover upward
	if isCrash {
		priceRecovery := (currentPrice - spikePrice) / spikePrice
		if priceRecovery > -0.005 { // Started recovering
			signal.Direction = "UP"
			signal.Confidence = 0.69
			signal.Reason = "Post-crash recovery phase"
			return signal
		}
	}

	// After boom spike, price should correct downward
	if isBoom {
		priceCorrection := (spikePrice - currentPrice) / spikePrice
		if priceCorrection > -0.005 { // Started correcting
			signal.Direction = "DOWN"
			signal.Confidence = 0.69
			signal.Reason = "Post-boom correction phase"
			return signal
		}
	}

	signal.Direction = "NONE"
	return signal
}

// calculateRecentVolatility calculates volatility over recent window
func (s *CrashBoomStrategy) calculateRecentVolatility(ticks []types.Tick, window int) float64 {
	if len(ticks) < window {
		return 0
	}

	recentTicks := ticks[len(ticks)-window:]

	// Calculate standard deviation of returns
	returns := make([]float64, len(recentTicks)-1)
	for i := 1; i < len(recentTicks); i++ {
		returns[i-1] = (recentTicks[i].Price - recentTicks[i-1].Price) / recentTicks[i-1].Price
	}

	// Mean
	sum := 0.0
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))

	// Variance
	variance := 0.0
	for _, r := range returns {
		variance += math.Pow(r-mean, 2)
	}
	variance /= float64(len(returns))

	return math.Sqrt(variance)
}

// IsCrashBoomMarket checks if market is crash/boom
func IsCrashBoomMarket(market string) bool {
	market = strings.ToLower(market)
	return strings.Contains(market, "crash") || strings.Contains(market, "boom")
}
