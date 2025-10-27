package strategy

import (
	"fmt"
	"math"
	"otc-predictor/internal/indicators"
	"otc-predictor/pkg/types"
)

// CombinedStrategy combines all strategies with STRICT quality control
type CombinedStrategy struct {
	volatilityStrategy *VolatilityStrategy
	crashBoomStrategy  *CrashBoomStrategy
	config             types.StrategyConfig
}

// NewCombinedStrategy creates a combined strategy
func NewCombinedStrategy(config types.StrategyConfig) *CombinedStrategy {
	return &CombinedStrategy{
		volatilityStrategy: NewVolatilityStrategy(config),
		crashBoomStrategy:  NewCrashBoomStrategy(config),
		config:             config,
	}
}

// GeneratePrediction generates final prediction using all strategies
func (s *CombinedStrategy) GeneratePrediction(market string, ticks []types.Tick, duration int) types.Prediction {
	prediction := types.Prediction{
		Market:     market,
		Direction:  "NONE",
		Duration:   duration,
		Timestamp:  ticks[len(ticks)-1].Timestamp,
		DataPoints: len(ticks),
	}

	// Dynamic minimum based on duration
	minRequired := s.getMinimumRequired(duration)

	if len(ticks) < minRequired {
		prediction.Reason = fmt.Sprintf("Collecting data: %d/%d ticks needed", len(ticks), minRequired)
		prediction.Confidence = 0
		return prediction
	}

	// Calculate indicators
	inds := indicators.CalculateAllIndicators(ticks, s.config)
	prediction.Indicators = inds
	prediction.CurrentPrice = ticks[len(ticks)-1].Price

	// Pre-filter: Skip if market conditions are poor
	if !s.isMarketTradeable(inds, ticks) {
		prediction.Reason = s.getMarketConditionReason(inds)
		prediction.Confidence = 0
		return prediction
	}

	// Collect signals from appropriate strategies
	var allSignals []types.StrategySignal

	if IsVolatilityMarket(market) {
		signals := s.volatilityStrategy.Analyze(ticks, inds)
		allSignals = append(allSignals, signals...)
	}

	if IsCrashBoomMarket(market) {
		signals := s.crashBoomStrategy.Analyze(ticks, inds, market)
		allSignals = append(allSignals, signals...)
	}

	// If no signals generated
	if len(allSignals) == 0 {
		prediction.Reason = "No trading signals generated"
		prediction.Confidence = 0
		return prediction
	}

	// Perform consensus voting with STRICT requirements
	return s.strictConsensusVoting(allSignals, prediction)
}

// getMinimumRequired returns dynamic minimum ticks
func (s *CombinedStrategy) getMinimumRequired(duration int) int {
	switch {
	case duration <= 30:
		return 50
	case duration <= 60:
		return 60
	case duration <= 120:
		return 75
	default:
		return 90
	}
}

// isMarketTradeable checks if market conditions allow trading
func (s *CombinedStrategy) isMarketTradeable(inds types.Indicators, ticks []types.Tick) bool {
	// Filter 1: Extreme volatility (market too unstable)
	maxVolatility := inds.BBMiddle * 0.025
	if inds.Volatility > maxVolatility {
		return false
	}

	// Filter 2: Dead zone - RSI between 45-55 with low momentum
	if inds.RSI > 45 && inds.RSI < 55 && math.Abs(inds.Momentum) < 0.001 {
		return false
	}

	// Filter 3: Check for data quality (no stale prices)
	if len(ticks) >= 10 {
		recentPrices := ticks[len(ticks)-10:]
		allSame := true
		firstPrice := recentPrices[0].Price
		for _, tick := range recentPrices {
			if math.Abs(tick.Price-firstPrice) > firstPrice*0.0001 {
				allSame = false
				break
			}
		}
		if allSame {
			return false // Stale/frozen prices
		}
	}

	// Filter 4: BB too wide (high uncertainty)
	bbWidth := (inds.BBUpper - inds.BBLower) / inds.BBMiddle
	if bbWidth > 0.05 {
		return false
	}

	return true
}

// getMarketConditionReason returns why market is not tradeable
func (s *CombinedStrategy) getMarketConditionReason(inds types.Indicators) string {
	maxVolatility := inds.BBMiddle * 0.025
	if inds.Volatility > maxVolatility {
		return fmt.Sprintf("Market too volatile (%.5f)", inds.Volatility)
	}

	if inds.RSI > 45 && inds.RSI < 55 && math.Abs(inds.Momentum) < 0.001 {
		return "Market in neutral zone - no clear direction"
	}

	bbWidth := (inds.BBUpper - inds.BBLower) / inds.BBMiddle
	if bbWidth > 0.05 {
		return "Bollinger Bands too wide - high uncertainty"
	}

	return "Poor market conditions for trading"
}

// strictConsensusVoting requires STRONG agreement between strategies
func (s *CombinedStrategy) strictConsensusVoting(signals []types.StrategySignal, basePrediction types.Prediction) types.Prediction {
	upVotes := 0
	downVotes := 0
	upConfidence := 0.0
	downConfidence := 0.0
	upWeight := 0.0
	downWeight := 0.0
	upReasons := []string{}
	downReasons := []string{}

	// Collect votes
	for _, signal := range signals {
		if signal.Direction == "UP" {
			upVotes++
			upConfidence += signal.Confidence * signal.Weight
			upWeight += signal.Weight
			upReasons = append(upReasons, fmt.Sprintf("%s(%.0f%%)", signal.Name, signal.Confidence*100))
		} else if signal.Direction == "DOWN" {
			downVotes++
			downConfidence += signal.Confidence * signal.Weight
			downWeight += signal.Weight
			downReasons = append(downReasons, fmt.Sprintf("%s(%.0f%%)", signal.Name, signal.Confidence*100))
		}
	}

	totalVotes := upVotes + downVotes
	if totalVotes == 0 {
		return s.noPrediction(basePrediction, "No directional signals")
	}

	// ULTRA-STRICT: Require at least 75% agreement (3:1 ratio)
	finalDirection := "NONE"
	finalConfidence := 0.0
	finalReasons := []string{}
	voteRatio := 0.0

	if upVotes > downVotes {
		voteRatio = float64(upVotes) / float64(totalVotes)
		if voteRatio >= 0.75 { // 75% agreement required
			finalDirection = "UP"
			finalConfidence = upConfidence / upWeight
			finalReasons = upReasons
		}
	} else if downVotes > upVotes {
		voteRatio = float64(downVotes) / float64(totalVotes)
		if voteRatio >= 0.75 { // 75% agreement required
			finalDirection = "DOWN"
			finalConfidence = downConfidence / downWeight
			finalReasons = downReasons
		}
	}

	// No strong consensus
	if finalDirection == "NONE" {
		reason := fmt.Sprintf("Weak consensus: %d UP vs %d DOWN (need 75%% agreement)", upVotes, downVotes)
		return s.noPrediction(basePrediction, reason)
	}

	// Apply ULTRA-STRICT quality filters
	filterResult, filterReason := s.ultraStrictFilters(basePrediction.Indicators, finalConfidence, len(signals), finalDirection)
	if !filterResult {
		return s.noPrediction(basePrediction, "Failed filters: "+filterReason)
	}

	// Final confidence check (STRICT)
	if finalConfidence < s.config.MinConfidence {
		reason := fmt.Sprintf("Confidence %.1f%% below %.1f%% threshold",
			finalConfidence*100, s.config.MinConfidence*100)
		return s.noPrediction(basePrediction, reason)
	}

	// All checks passed - return prediction
	basePrediction.Direction = finalDirection
	basePrediction.Confidence = finalConfidence
	basePrediction.Reason = fmt.Sprintf("%d/%d strategies agree %s (%.0f%%): %v",
		int(voteRatio*float64(totalVotes)), totalVotes, finalDirection, voteRatio*100, finalReasons)

	return basePrediction
}

// ultraStrictFilters applies the strictest quality checks
func (s *CombinedStrategy) ultraStrictFilters(inds types.Indicators, confidence float64, signalCount int, direction string) (bool, string) {
	// Filter 1: Minimum confidence (RAISED)
	if confidence < 0.70 {
		return false, fmt.Sprintf("confidence %.1f%% too low", confidence*100)
	}

	// Filter 2: Minimum number of agreeing strategies
	if signalCount < 2 {
		return false, "need at least 2 agreeing strategies"
	}

	// Filter 3: EMA alignment check (trend must be clear)
	emaSpread := math.Abs(inds.EMA9 - inds.EMA21)
	minSpread := inds.BBMiddle * 0.0015
	if emaSpread < minSpread {
		return false, "EMAs too close - no clear trend"
	}

	// Filter 4: RSI must not be in extreme danger zone
	if direction == "UP" && inds.RSI > 70 {
		return false, fmt.Sprintf("RSI too high for UP (%.1f)", inds.RSI)
	}
	if direction == "DOWN" && inds.RSI < 30 {
		return false, fmt.Sprintf("RSI too low for DOWN (%.1f)", inds.RSI)
	}

	// Filter 5: BB position should support the direction
	if direction == "UP" && inds.BBPosition > 0.5 {
		return false, "price too high in BB range for UP"
	}
	if direction == "DOWN" && inds.BBPosition < -0.5 {
		return false, "price too low in BB range for DOWN"
	}

	// Filter 6: Momentum must support direction
	if direction == "UP" && inds.Momentum < -0.002 {
		return false, "momentum contradicts UP signal"
	}
	if direction == "DOWN" && inds.Momentum > 0.002 {
		return false, "momentum contradicts DOWN signal"
	}

	// Filter 7: Volatility must be reasonable
	maxVol := inds.BBMiddle * 0.03
	if inds.Volatility > maxVol {
		return false, "volatility too high"
	}

	return true, ""
}

// noPrediction creates a "NONE" prediction with reason
func (s *CombinedStrategy) noPrediction(base types.Prediction, reason string) types.Prediction {
	base.Direction = "NONE"
	base.Confidence = 0
	base.Reason = reason
	return base
}
