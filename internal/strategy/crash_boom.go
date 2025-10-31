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
// 🔧 FIXED: Adjusted all confidence levels to match new 58% threshold
func (s *CrashBoomStrategy) Analyze(ticks []types.Tick, inds types.Indicators, market string) []types.StrategySignal {
	signals := []types.StrategySignal{}

	if len(ticks) < 50 { // Was 60 - reduced minimum
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

	// Strategy 3: Pre-spike volatility indicator
	preSpikeSignal := s.preSpikeVolatilitySignal(ticks, inds, spikeStats, isCrash, isBoom)
	if preSpikeSignal.Direction != "NONE" {
		signals = append(signals, preSpikeSignal)
	}

	// Strategy 4: Post-spike recovery
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
// 🔧 FIXED: Adjusted confidence calculations
func (s *CrashBoomStrategy) analyzeSpikePattern(ticks []types.Tick) SpikeStats {
	stats := SpikeStats{
		LastSpikeIdx: -1,
		RecentSpikes: []int{},
	}

	if len(ticks) < 20 {
		return stats
	}

	// Detect spikes
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

		// ✅ FIXED: Relaxed spike due window (85-110% → 80-115%)
		progress := float64(stats.TicksSinceSpike) / stats.AvgInterval
		if progress >= 0.80 && progress <= 1.15 {
			stats.SpikeDue = true
			// ✅ Adjusted confidence range (0.55-0.75 → 0.50-0.70)
			stats.Confidence = math.Min(0.70, 0.50+(progress-0.80)*0.57)
		}
	} else {
		// Only one spike found
		stats.AvgInterval = 70.0
		if stats.TicksSinceSpike >= 55 { // Was 60
			stats.SpikeDue = true
			stats.Confidence = 0.58 // Was 0.62
		}
	}

	return stats
}

// isSpikePoint detects if a point is a spike
// 🔧 FIXED: Slightly relaxed detection (2.5% → 2.2%)
func (s *CrashBoomStrategy) isSpikePoint(ticks []types.Tick, idx int) bool {
	if idx < 5 || idx >= len(ticks)-5 {
		return false
	}

	currentPrice := ticks[idx].Price

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

	changeFromBefore := math.Abs((currentPrice - beforeAvg) / beforeAvg)
	changeToAfter := math.Abs((afterAvg - currentPrice) / currentPrice)
	immediateChange := math.Abs((currentPrice - ticks[idx-1].Price) / ticks[idx-1].Price)

	// ✅ FIXED: Relaxed from 0.025/0.015 to 0.022/0.013
	return (changeFromBefore > 0.022 || changeToAfter > 0.022) && immediateChange > 0.013
}

// spikeDetectionSignal predicts when spike is imminent
// 🔧 FIXED: Adjusted confidence levels
func (s *CrashBoomStrategy) spikeDetectionSignal(ticks []types.Tick, stats SpikeStats, isCrash, isBoom bool) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "SpikeDetection",
		Weight: 0.50,
	}

	if !stats.SpikeDue {
		signal.Direction = "NONE"
		return signal
	}

	recentVolatility := s.calculateRecentVolatility(ticks, 10)
	baseline := s.calculateRecentVolatility(ticks, 30)

	volatilityIncreasing := recentVolatility > baseline*1.12 // Was 1.15

	confidence := stats.Confidence

	if volatilityIncreasing {
		confidence += 0.05 // Was 0.06
	}

	// ✅ FIXED: Relaxed from 0.95 to 0.92
	if stats.TicksSinceSpike >= int(stats.AvgInterval*0.92) {
		confidence += 0.04
	}

	if isCrash {
		signal.Direction = "DOWN"
		signal.Confidence = math.Min(0.74, confidence) // Was 0.78
		signal.Reason = "Crash spike imminent (pattern-based)"
		return signal
	}

	if isBoom {
		signal.Direction = "UP"
		signal.Confidence = math.Min(0.74, confidence) // Was 0.78
		signal.Reason = "Boom spike imminent (pattern-based)"
		return signal
	}

	signal.Direction = "NONE"
	return signal
}

// betweenSpikeTrendSignal for the drift period between spikes
// 🔧 FIXED: Adjusted confidence and relaxed conditions
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

	// ✅ FIXED: Expanded window (20-75% → 15-80%)
	if progress < 0.15 || progress > 0.80 {
		signal.Direction = "NONE"
		return signal
	}

	// Crash indices: Rise between crashes
	if isCrash && inds.RSI < 68 && inds.Momentum >= -0.0015 { // Was 65 / -0.001
		confidence := 0.62 // Was 0.66

		// Stronger confidence in mid-range
		if progress >= 0.30 && progress <= 0.65 { // Was 0.35-0.60
			confidence = 0.66 // Was 0.70
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

	// Boom indices: Fall between booms
	if isBoom && inds.RSI > 32 && inds.Momentum <= 0.0015 { // Was 35 / 0.001
		confidence := 0.62 // Was 0.66

		// Stronger confidence in mid-range
		if progress >= 0.30 && progress <= 0.65 {
			confidence = 0.66 // Was 0.70
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

// preSpikeVolatilitySignal - Detects volatility changes before spike
// 🔧 FIXED: Adjusted confidence and thresholds
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

	// ✅ FIXED: Expanded window (80-105% → 75-110%)
	if progress < 0.75 || progress > 1.10 {
		signal.Direction = "NONE"
		return signal
	}

	recentVol := s.calculateRecentVolatility(ticks, 15)
	baselineVol := s.calculateRecentVolatility(ticks, 50)

	// ✅ FIXED: Relaxed from 1.25 to 1.20
	if recentVol > baselineVol*1.20 {
		confidence := 0.64 // Was 0.68

		// ✅ FIXED: Relaxed from 1.5 to 1.4
		if recentVol > baselineVol*1.4 {
			confidence = 0.68 // Was 0.72
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

// postSpikeRecoverySignal - Trade the recovery after spike
// 🔧 FIXED: Adjusted confidence
func (s *CrashBoomStrategy) postSpikeRecoverySignal(ticks []types.Tick, stats SpikeStats, isCrash, isBoom bool) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "PostSpikeRecovery",
		Weight: 0.20,
	}

	if stats.LastSpikeIdx == -1 {
		signal.Direction = "NONE"
		return signal
	}

	// ✅ FIXED: Expanded window (1-10 → 1-12 ticks)
	if stats.TicksSinceSpike < 1 || stats.TicksSinceSpike > 12 {
		signal.Direction = "NONE"
		return signal
	}

	if stats.LastSpikeIdx >= len(ticks)-1 {
		signal.Direction = "NONE"
		return signal
	}

	spikePrice := ticks[stats.LastSpikeIdx].Price
	currentPrice := ticks[len(ticks)-1].Price

	// After crash spike, price should recover upward
	if isCrash {
		priceRecovery := (currentPrice - spikePrice) / spikePrice
		if priceRecovery > -0.008 { // Was -0.005 (more lenient)
			signal.Direction = "UP"
			signal.Confidence = 0.65 // Was 0.69
			signal.Reason = "Post-crash recovery phase"
			return signal
		}
	}

	// After boom spike, price should correct downward
	if isBoom {
		priceCorrection := (spikePrice - currentPrice) / spikePrice
		if priceCorrection > -0.008 { // Was -0.005
			signal.Direction = "DOWN"
			signal.Confidence = 0.65 // Was 0.69
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

	returns := make([]float64, len(recentTicks)-1)
	for i := 1; i < len(recentTicks); i++ {
		returns[i-1] = (recentTicks[i].Price - recentTicks[i-1].Price) / recentTicks[i-1].Price
	}

	sum := 0.0
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))

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
