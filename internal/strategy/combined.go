package strategy

import (
	"fmt"
	"math"
	"otc-predictor/internal/indicators"
	"otc-predictor/pkg/types"
)

// CombinedStrategy combines all strategies with mode awareness
type CombinedStrategy struct {
	volatilityStrategy *VolatilityStrategy
	crashBoomStrategy  *CrashBoomStrategy
	forexStrategy      *ForexStrategy
	config             types.StrategyConfig
}

// NewCombinedStrategy creates a combined strategy
func NewCombinedStrategy(config types.StrategyConfig) *CombinedStrategy {
	return &CombinedStrategy{
		volatilityStrategy: NewVolatilityStrategy(config),
		crashBoomStrategy:  NewCrashBoomStrategy(config),
		forexStrategy:      NewForexStrategy(config),
		config:             config,
	}
}

// GeneratePrediction generates prediction with market-type awareness
func (s *CombinedStrategy) GeneratePrediction(market string, ticks []types.Tick, duration int) types.Prediction {
	prediction := types.Prediction{
		Market:     market,
		Direction:  "NONE",
		Duration:   duration,
		Timestamp:  ticks[len(ticks)-1].Timestamp,
		DataPoints: len(ticks),
	}

	// Determine market type
	marketType := s.getMarketType(market)
	minRequired := s.getMinimumRequired(duration, marketType)

	if len(ticks) < minRequired {
		prediction.Reason = fmt.Sprintf("Collecting data: %d/%d ticks needed", len(ticks), minRequired)
		prediction.Confidence = 0
		return prediction
	}

	// Calculate indicators
	inds := indicators.CalculateAllIndicators(ticks, s.config)
	prediction.Indicators = inds
	prediction.CurrentPrice = ticks[len(ticks)-1].Price

	// Pre-filter: Skip poor market conditions
	if !s.isMarketTradeable(inds, ticks, marketType) {
		prediction.Reason = s.getMarketConditionReason(inds, marketType)
		prediction.Confidence = 0
		return prediction
	}

	// Collect signals based on market type
	var allSignals []types.StrategySignal

	switch marketType {
	case "volatility":
		allSignals = s.volatilityStrategy.Analyze(ticks, inds)

	case "crash_boom":
		allSignals = s.crashBoomStrategy.Analyze(ticks, inds, market)

	case "forex":
		allSignals = s.forexStrategy.Analyze(ticks, inds)

	default:
		prediction.Reason = "Unknown market type"
		return prediction
	}

	if len(allSignals) == 0 {
		prediction.Reason = "No trading signals generated"
		prediction.Confidence = 0
		return prediction
	}

	// Use market-specific consensus rules
	return s.marketAwareConsensus(allSignals, prediction, marketType)
}

// getMarketType determines what type of market this is
func (s *CombinedStrategy) getMarketType(market string) string {
	if IsForexMarket(market) {
		return "forex"
	}
	if IsCrashBoomMarket(market) {
		return "crash_boom"
	}
	if IsVolatilityMarket(market) {
		return "volatility"
	}
	return "unknown"
}

// getMinimumRequired returns minimum ticks based on market type and duration
func (s *CombinedStrategy) getMinimumRequired(duration int, marketType string) int {
	// Forex needs more data (slower moving + longer durations)
	if marketType == "forex" {
		// Forex minimum duration is 15 minutes (900s)
		switch {
		case duration <= 900: // 15 minutes
			return 200
		case duration <= 1800: // 30 minutes
			return 250
		default: // 1 hour+
			return 300
		}
	}

	// Synthetics can work with less data (faster moving)
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

// isMarketTradeable checks if conditions allow trading (market-aware)
func (s *CombinedStrategy) isMarketTradeable(inds types.Indicators, ticks []types.Tick, marketType string) bool {
	// Common filters
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
			return false // Stale prices
		}
	}

	// Market-specific filters
	if marketType == "forex" {
		// Forex: stricter volatility limits
		maxVolatility := inds.BBMiddle * 0.02
		if inds.Volatility > maxVolatility {
			return false
		}

		// Forex: avoid dead zone more strictly
		if inds.RSI > 48 && inds.RSI < 52 && math.Abs(inds.Momentum) < 0.0008 {
			return false
		}

		// Forex: BB shouldn't be too wide
		bbWidth := (inds.BBUpper - inds.BBLower) / inds.BBMiddle
		if bbWidth > 0.04 {
			return false
		}
	} else {
		// Synthetics: different limits
		maxVolatility := inds.BBMiddle * 0.025
		if inds.Volatility > maxVolatility {
			return false
		}

		if inds.RSI > 45 && inds.RSI < 55 && math.Abs(inds.Momentum) < 0.001 {
			return false
		}

		bbWidth := (inds.BBUpper - inds.BBLower) / inds.BBMiddle
		if bbWidth > 0.05 {
			return false
		}
	}

	return true
}

// getMarketConditionReason returns reason for poor conditions
func (s *CombinedStrategy) getMarketConditionReason(inds types.Indicators, marketType string) string {
	if marketType == "forex" {
		maxVolatility := inds.BBMiddle * 0.02
		if inds.Volatility > maxVolatility {
			return fmt.Sprintf("Forex volatility too high (%.5f)", inds.Volatility)
		}

		if inds.RSI > 48 && inds.RSI < 52 {
			return "Forex market in neutral zone"
		}

		bbWidth := (inds.BBUpper - inds.BBLower) / inds.BBMiddle
		if bbWidth > 0.04 {
			return "Forex BB too wide - uncertain market"
		}
	}

	return "Poor market conditions for trading"
}

// marketAwareConsensus applies market-specific consensus rules
func (s *CombinedStrategy) marketAwareConsensus(signals []types.StrategySignal, basePrediction types.Prediction, marketType string) types.Prediction {
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

	// Market-specific consensus requirements
	requiredAgreement := 0.75 // Default: 75%
	if marketType == "forex" {
		requiredAgreement = 0.70 // Forex: 70% (slightly more lenient due to clearer trends)
	}

	finalDirection := "NONE"
	finalConfidence := 0.0
	finalReasons := []string{}
	voteRatio := 0.0

	if upVotes > downVotes {
		voteRatio = float64(upVotes) / float64(totalVotes)
		if voteRatio >= requiredAgreement {
			finalDirection = "UP"
			finalConfidence = upConfidence / upWeight
			finalReasons = upReasons
		}
	} else if downVotes > upVotes {
		voteRatio = float64(downVotes) / float64(totalVotes)
		if voteRatio >= requiredAgreement {
			finalDirection = "DOWN"
			finalConfidence = downConfidence / downWeight
			finalReasons = downReasons
		}
	}

	// No consensus
	if finalDirection == "NONE" {
		reason := fmt.Sprintf("Weak consensus: %d UP vs %d DOWN (need %.0f%% agreement)",
			upVotes, downVotes, requiredAgreement*100)
		return s.noPrediction(basePrediction, reason)
	}

	// Apply market-specific quality filters
	filterResult, filterReason := s.marketSpecificFilters(basePrediction.Indicators, finalConfidence,
		len(signals), finalDirection, marketType)
	if !filterResult {
		return s.noPrediction(basePrediction, "Failed filters: "+filterReason)
	}

	// Final confidence check
	minConfidence := s.config.MinConfidence
	if marketType == "forex" {
		minConfidence = 0.68 // Slightly lower for forex (68% vs 70%)
	}

	if finalConfidence < minConfidence {
		reason := fmt.Sprintf("Confidence %.1f%% below %.1f%% threshold",
			finalConfidence*100, minConfidence*100)
		return s.noPrediction(basePrediction, reason)
	}

	// Success - return prediction
	basePrediction.Direction = finalDirection
	basePrediction.Confidence = finalConfidence
	basePrediction.Reason = fmt.Sprintf("[%s] %d/%d strategies agree %s (%.0f%%): %v",
		marketType, int(voteRatio*float64(totalVotes)), totalVotes, finalDirection,
		voteRatio*100, finalReasons)

	return basePrediction
}

// marketSpecificFilters applies filters based on market type
func (s *CombinedStrategy) marketSpecificFilters(inds types.Indicators, confidence float64,
	signalCount int, direction string, marketType string) (bool, string) {

	// Filter 1: Minimum signal count
	minSignals := 2
	if signalCount < minSignals {
		return false, fmt.Sprintf("need at least %d agreeing strategies", minSignals)
	}

	// Filter 2: Base confidence check
	minConf := 0.68
	if marketType == "synthetics" {
		minConf = 0.70
	}
	if confidence < minConf {
		return false, fmt.Sprintf("confidence %.1f%% too low", confidence*100)
	}

	// Filter 3: EMA alignment (universal)
	emaSpread := math.Abs(inds.EMA9 - inds.EMA21)
	minSpread := inds.BBMiddle * 0.0015
	if emaSpread < minSpread {
		return false, "EMAs too close - no clear trend"
	}

	// Market-specific filters
	if marketType == "forex" {
		// Forex Filter 1: RSI zones (more lenient)
		if direction == "UP" && inds.RSI > 72 {
			return false, fmt.Sprintf("RSI too high for forex UP (%.1f)", inds.RSI)
		}
		if direction == "DOWN" && inds.RSI < 28 {
			return false, fmt.Sprintf("RSI too low for forex DOWN (%.1f)", inds.RSI)
		}

		// Forex Filter 2: Momentum check
		if direction == "UP" && inds.Momentum < -0.003 {
			return false, "momentum contradicts forex UP signal"
		}
		if direction == "DOWN" && inds.Momentum > 0.003 {
			return false, "momentum contradicts forex DOWN signal"
		}

		// Forex Filter 3: Volatility
		maxVol := inds.BBMiddle * 0.025
		if inds.Volatility > maxVol {
			return false, "forex volatility too high"
		}

	} else {
		// Synthetics Filter 1: Stricter RSI zones
		if direction == "UP" && inds.RSI > 70 {
			return false, fmt.Sprintf("RSI too high for synthetic UP (%.1f)", inds.RSI)
		}
		if direction == "DOWN" && inds.RSI < 30 {
			return false, fmt.Sprintf("RSI too low for synthetic DOWN (%.1f)", inds.RSI)
		}

		// Synthetics Filter 2: BB position
		if direction == "UP" && inds.BBPosition > 0.5 {
			return false, "price too high in BB range for UP"
		}
		if direction == "DOWN" && inds.BBPosition < -0.5 {
			return false, "price too low in BB range for DOWN"
		}

		// Synthetics Filter 3: Momentum
		if direction == "UP" && inds.Momentum < -0.002 {
			return false, "momentum contradicts synthetic UP signal"
		}
		if direction == "DOWN" && inds.Momentum > 0.002 {
			return false, "momentum contradicts synthetic DOWN signal"
		}

		// Synthetics Filter 4: Volatility
		maxVol := inds.BBMiddle * 0.03
		if inds.Volatility > maxVol {
			return false, "synthetic volatility too high"
		}
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
