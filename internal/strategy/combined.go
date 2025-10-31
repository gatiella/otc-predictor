package strategy

import (
	"fmt"
	"math"
	"otc-predictor/internal/candles"
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
		prediction.Reason = fmt.Sprintf("Collecting data: %d/%d ticks needed (~%d min)",
			len(ticks), minRequired, (minRequired-len(ticks))/2)
		prediction.Confidence = 0
		return prediction
	}

	// Calculate indicators
	inds := indicators.CalculateAllIndicators(ticks, s.config)
	prediction.Indicators = inds
	prediction.CurrentPrice = ticks[len(ticks)-1].Price

	// ✨ Check for RSI Divergence (very reliable signal)
	divergence := indicators.DetectRSIDivergence(ticks, s.config)

	// ✨ Check for advanced patterns
	pattern := indicators.DetectAdvancedPatterns(ticks)

	// ✅ ULTRA-RELAXED pre-filter - only block extreme chaos
	if !s.isMarketTradeable(inds, ticks, marketType) {
		prediction.Reason = s.getMarketConditionReason(inds, marketType)
		prediction.Confidence = 0
		return prediction
	}

	// Collect signals based on market type
	var allSignals []types.StrategySignal

	// ✨ Add divergence signal if detected (HIGHEST PRIORITY)
	if divergence.Type != indicators.NoDivergence {
		divSignal := s.createDivergenceSignal(divergence)
		if divSignal.Direction != "NONE" {
			allSignals = append(allSignals, divSignal)
		}
	}

	// ✨ Add pattern signal if detected (HIGH PRIORITY)
	if pattern.Type != indicators.NoPattern {
		patternSignal := s.createPatternSignal(pattern)
		if patternSignal.Direction != "NONE" {
			allSignals = append(allSignals, patternSignal)
		}
	}

	switch marketType {
	case "volatility":
		allSignals = append(allSignals, s.volatilityStrategy.Analyze(ticks, inds)...)

	case "crash_boom":
		allSignals = append(allSignals, s.crashBoomStrategy.Analyze(ticks, inds, market)...)

	case "forex":
		tfConfig := candles.GetTimeframeConfig(duration, "forex")
		allSignals = append(allSignals, s.forexStrategy.AnalyzeWithTimeframe(ticks, inds, tfConfig, duration)...)

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
// ✅ ULTRA-FAST: Absolute minimum for signals (safety first, speed second)
func (s *CombinedStrategy) getMinimumRequired(duration int, marketType string) int {
	if marketType == "forex" {
		// ✅ DRASTICALLY REDUCED: Get signals FAST
		switch {
		case duration <= 900: // 15 minutes
			return 20 // Was 30 - now even faster (10 min wait)
		case duration <= 1800: // 30 minutes
			return 25 // Was 40
		default: // 1 hour+
			return 30 // Was 50
		}
	}

	// Synthetics - also very fast
	switch {
	case duration <= 30:
		return 20 // Was 30
	case duration <= 60:
		return 25 // Was 40
	case duration <= 120:
		return 30 // Was 50
	default:
		return 35 // Was 60
	}
}

// isMarketTradeable checks if conditions allow trading (market-aware)
// ✅ ULTRA-RELAXED: Only block complete market chaos
func (s *CombinedStrategy) isMarketTradeable(inds types.Indicators, ticks []types.Tick, marketType string) bool {
	// Check for stale prices (critical safety check)
	if len(ticks) >= 8 {
		recentPrices := ticks[len(ticks)-8:]
		allSame := true
		firstPrice := recentPrices[0].Price
		for _, tick := range recentPrices {
			if math.Abs(tick.Price-firstPrice) > firstPrice*0.00005 {
				allSame = false
				break
			}
		}
		if allSame {
			return false // Market frozen - critical issue
		}
	}

	// ✅ ULTRA-RELAXED: Only block absolute market chaos
	if marketType == "forex" {
		// Only block EXTREME volatility (>10% of price)
		maxVolatility := inds.BBMiddle * 0.10 // Was 0.08
		if inds.Volatility > maxVolatility {
			return false
		}
	} else {
		// Synthetics: very high tolerance
		maxVolatility := inds.BBMiddle * 0.08 // Was 0.06
		if inds.Volatility > maxVolatility {
			return false
		}
	}

	return true
}

// getMarketConditionReason returns reason for poor conditions
func (s *CombinedStrategy) getMarketConditionReason(inds types.Indicators, marketType string) string {
	if marketType == "forex" {
		maxVolatility := inds.BBMiddle * 0.10
		if inds.Volatility > maxVolatility {
			return fmt.Sprintf("Extreme market volatility (%.5f) - too dangerous", inds.Volatility)
		}
	}

	return "Market conditions too unstable"
}

// marketAwareConsensus applies market-specific consensus rules
// ✅ ULTRA-FAST: Just need ANY reasonable signal
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

	// ✅ ULTRA-FAST: Accept ANY single clear signal (no consensus needed)
	requiredAgreement := 0.40 // Just 40% - even minority signals count!

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

	// No signal at all
	if finalDirection == "NONE" {
		reason := fmt.Sprintf("Conflicting signals: %d UP vs %d DOWN", upVotes, downVotes)
		return s.noPrediction(basePrediction, reason)
	}

	// ✅ ULTRA-LOW threshold: Accept weak signals too
	minConfidence := 0.52 // Was 0.55 - now MUCH lower

	if finalConfidence < minConfidence {
		reason := fmt.Sprintf("Confidence %.1f%% below %.1f%% minimum",
			finalConfidence*100, minConfidence*100)
		return s.noPrediction(basePrediction, reason)
	}

	// Success - return prediction
	basePrediction.Direction = finalDirection
	basePrediction.Confidence = finalConfidence
	basePrediction.Reason = fmt.Sprintf("[%s] %d/%d strategies %s (%.0f%% agreement): %v",
		marketType, int(voteRatio*float64(totalVotes)), totalVotes, finalDirection,
		voteRatio*100, finalReasons)

	return basePrediction
}

// noPrediction creates a "NONE" prediction with reason
func (s *CombinedStrategy) noPrediction(base types.Prediction, reason string) types.Prediction {
	base.Direction = "NONE"
	base.Confidence = 0
	base.Reason = reason
	return base
}

// createDivergenceSignal converts divergence to strategy signal
func (s *CombinedStrategy) createDivergenceSignal(div indicators.Divergence) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "RSI_Divergence",
		Weight: 0.55, // HIGHEST WEIGHT
	}

	switch div.Type {
	case indicators.BullishDivergence:
		signal.Direction = "UP"
		signal.Confidence = div.Confidence
		signal.Reason = fmt.Sprintf("Bullish RSI divergence (%.0f%% strength)", div.Strength*100)

	case indicators.BearishDivergence:
		signal.Direction = "DOWN"
		signal.Confidence = div.Confidence
		signal.Reason = fmt.Sprintf("Bearish RSI divergence (%.0f%% strength)", div.Strength*100)

	case indicators.HiddenBullishDivergence:
		signal.Direction = "UP"
		signal.Confidence = div.Confidence
		signal.Reason = "Hidden bullish divergence (trend continuation)"

	case indicators.HiddenBearishDivergence:
		signal.Direction = "DOWN"
		signal.Confidence = div.Confidence
		signal.Reason = "Hidden bearish divergence (trend continuation)"

	default:
		signal.Direction = "NONE"
	}

	return signal
}

// createPatternSignal converts pattern to strategy signal
func (s *CombinedStrategy) createPatternSignal(pattern indicators.Pattern) types.StrategySignal {
	signal := types.StrategySignal{
		Name:       "Advanced_Pattern",
		Direction:  pattern.Direction,
		Confidence: pattern.Confidence,
		Weight:     0.45, // HIGH WEIGHT
	}

	patternNames := map[indicators.PatternType]string{
		indicators.HeadAndShoulders:        "Head & Shoulders",
		indicators.InverseHeadAndShoulders: "Inverse H&S",
		indicators.DoubleTop:               "Double Top",
		indicators.DoubleBottom:            "Double Bottom",
		indicators.TripleTop:               "Triple Top",
		indicators.TripleBottom:            "Triple Bottom",
		indicators.AscendingTriangle:       "Ascending Triangle",
		indicators.DescendingTriangle:      "Descending Triangle",
	}

	if name, exists := patternNames[pattern.Type]; exists {
		signal.Reason = fmt.Sprintf("%s pattern detected (%.0f%% quality)", name, pattern.Strength*100)
	} else {
		signal.Reason = "Chart pattern detected"
	}

	return signal
}
