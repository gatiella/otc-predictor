package strategy

import (
	"otc-predictor/internal/indicators"
	"otc-predictor/pkg/types"
	"strings"
)

// VolatilityStrategy for Volatility indices (V10, V25, V50, V75, V100)
type VolatilityStrategy struct {
	config types.StrategyConfig
}

// NewVolatilityStrategy creates a new volatility strategy
func NewVolatilityStrategy(config types.StrategyConfig) *VolatilityStrategy {
	return &VolatilityStrategy{
		config: config,
	}
}

// Analyze generates signals for volatility indices
func (s *VolatilityStrategy) Analyze(ticks []types.Tick, inds types.Indicators) []types.StrategySignal {
	signals := []types.StrategySignal{}

	if len(ticks) < s.config.RSIPeriod*2 {
		return signals
	}

	currentPrice := ticks[len(ticks)-1].Price
	pattern := indicators.DetectPattern(ticks)

	// Strategy 1: Mean Reversion (STRONGEST for volatility indices)
	meanReversionSignal := s.meanReversionSignal(currentPrice, inds, pattern)
	if meanReversionSignal.Direction != "NONE" {
		signals = append(signals, meanReversionSignal)
	}

	// Strategy 2: Momentum Following (when trend is clear)
	momentumSignal := s.momentumSignal(inds)
	if momentumSignal.Direction != "NONE" {
		signals = append(signals, momentumSignal)
	}

	// Strategy 3: Bollinger Band Extremes (mean reversion at bands)
	bbSignal := s.bollingerBandSignal(currentPrice, inds)
	if bbSignal.Direction != "NONE" {
		signals = append(signals, bbSignal)
	}

	// Strategy 4: RSI Extremes with confirmation
	rsiSignal := s.rsiSignal(inds, ticks)
	if rsiSignal.Direction != "NONE" {
		signals = append(signals, rsiSignal)
	}

	// Strategy 5: EMA Crossover (strong signal)
	crossoverSignal := s.emaCrossoverSignal(ticks, inds)
	if crossoverSignal.Direction != "NONE" {
		signals = append(signals, crossoverSignal)
	}

	return signals
}

// meanReversionSignal - Price returns to mean (HIGHEST WEIGHT)
// 🔧 FIXED: Adjusted confidence to match new 58% threshold
func (s *VolatilityStrategy) meanReversionSignal(price float64, inds types.Indicators, pattern string) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "MeanReversion",
		Weight: 0.45,
	}

	// ✅ FIXED: Relaxed from -0.75 to -0.70 for more signals
	if inds.RSI < s.config.RSIOversold && inds.BBPosition < -0.70 {
		confidence := 0.68 // Was 0.72 - adjusted down

		// Pattern confirmation
		if pattern == "double_bottom" {
			confidence = 0.74 // Was 0.78
		}

		// Extremely oversold
		if inds.RSI < 22 && inds.BBPosition < -0.85 {
			confidence += 0.05 // Was 0.06
		}

		// Momentum turning positive
		if inds.Momentum > -0.0008 { // Was -0.0005 (more lenient)
			confidence += 0.03
		}

		signal.Direction = "UP"
		signal.Confidence = confidence
		signal.Reason = "Strong oversold mean reversion signal"
		return signal
	}

	// ✅ FIXED: Relaxed from 0.75 to 0.70 for more signals
	if inds.RSI > s.config.RSIOverbought && inds.BBPosition > 0.70 {
		confidence := 0.68 // Was 0.72

		// Pattern confirmation
		if pattern == "double_top" {
			confidence = 0.74 // Was 0.78
		}

		// Extremely overbought
		if inds.RSI > 78 && inds.BBPosition > 0.85 {
			confidence += 0.05 // Was 0.06
		}

		// Momentum turning negative
		if inds.Momentum < 0.0008 { // Was 0.0005
			confidence += 0.03
		}

		signal.Direction = "DOWN"
		signal.Confidence = confidence
		signal.Reason = "Strong overbought mean reversion signal"
		return signal
	}

	signal.Direction = "NONE"
	return signal
}

// momentumSignal - Clear trend following
// 🔧 FIXED: Adjusted for new confidence threshold
func (s *VolatilityStrategy) momentumSignal(inds types.Indicators) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "Momentum",
		Weight: 0.35,
	}

	// ✅ FIXED: Relaxed RSI range (48-62 → 45-65)
	if inds.EMA9 > inds.EMA21 && inds.EMA21 > inds.EMA50 &&
		inds.RSI < 65 && inds.RSI > 45 && inds.Momentum > 0.0015 { // Was 0.002

		confidence := 0.64 // Was 0.68

		// Strong trend confirmation
		if inds.TrendStrength > 0.70 { // Was 0.75
			confidence = 0.69 // Was 0.73
		}

		// Price above all EMAs
		emaDistance := (inds.EMA9 - inds.EMA50) / inds.EMA50
		if emaDistance > 0.0025 { // Was 0.003
			confidence += 0.04
		}

		signal.Direction = "UP"
		signal.Confidence = confidence
		signal.Reason = "Strong bullish momentum alignment"
		return signal
	}

	// ✅ FIXED: Relaxed RSI range (38-52 → 35-55)
	if inds.EMA9 < inds.EMA21 && inds.EMA21 < inds.EMA50 &&
		inds.RSI > 35 && inds.RSI < 55 && inds.Momentum < -0.0015 { // Was -0.002

		confidence := 0.64 // Was 0.68

		// Strong trend confirmation
		if inds.TrendStrength > 0.70 { // Was 0.75
			confidence = 0.69 // Was 0.73
		}

		// Price below all EMAs
		emaDistance := (inds.EMA50 - inds.EMA9) / inds.EMA50
		if emaDistance > 0.0025 { // Was 0.003
			confidence += 0.04
		}

		signal.Direction = "DOWN"
		signal.Confidence = confidence
		signal.Reason = "Strong bearish momentum alignment"
		return signal
	}

	signal.Direction = "NONE"
	return signal
}

// bollingerBandSignal - Trading at extreme bands
// 🔧 FIXED: Adjusted thresholds
func (s *VolatilityStrategy) bollingerBandSignal(price float64, inds types.Indicators) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "BollingerBands",
		Weight: 0.25,
	}

	// ✅ FIXED: Relaxed from -0.85 to -0.80
	if inds.BBPosition < -0.80 && price < inds.BBLower {
		confidence := 0.65 // Was 0.69

		// RSI confirms oversold
		if inds.RSI < 35 {
			confidence += 0.05
		}

		signal.Direction = "UP"
		signal.Confidence = confidence
		signal.Reason = "Price at lower BB band - bounce expected"
		return signal
	}

	// ✅ FIXED: Relaxed from 0.85 to 0.80
	if inds.BBPosition > 0.80 && price > inds.BBUpper {
		confidence := 0.65 // Was 0.69

		// RSI confirms overbought
		if inds.RSI > 65 {
			confidence += 0.05
		}

		signal.Direction = "DOWN"
		signal.Confidence = confidence
		signal.Reason = "Price at upper BB band - pullback expected"
		return signal
	}

	// BB Squeeze breakout
	bbWidth := (inds.BBUpper - inds.BBLower) / inds.BBMiddle
	if bbWidth < 0.018 { // Was 0.015 - more lenient
		if price > inds.BBMiddle && inds.Momentum > 0.0025 { // Was 0.003
			signal.Direction = "UP"
			signal.Confidence = 0.62 // Was 0.66
			signal.Reason = "BB squeeze breakout upward"
			return signal
		}

		if price < inds.BBMiddle && inds.Momentum < -0.0025 { // Was -0.003
			signal.Direction = "DOWN"
			signal.Confidence = 0.62 // Was 0.66
			signal.Reason = "BB squeeze breakout downward"
			return signal
		}
	}

	signal.Direction = "NONE"
	return signal
}

// rsiSignal - RSI at true extremes with confirmation
// 🔧 FIXED: Adjusted for new threshold
func (s *VolatilityStrategy) rsiSignal(inds types.Indicators, ticks []types.Tick) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "RSI",
		Weight: 0.15,
	}

	// Check RSI trend (is it turning?)
	rsiTurning := false
	if len(ticks) > s.config.RSIPeriod+5 {
		prevInds := indicators.CalculateAllIndicators(ticks[:len(ticks)-5], s.config)
		if inds.RSI < 28 && prevInds.RSI < inds.RSI { // Was 25
			rsiTurning = true
		}
		if inds.RSI > 72 && prevInds.RSI > inds.RSI { // Was 75
			rsiTurning = true
		}
	}

	// ✅ FIXED: Relaxed from 25 to 28
	if inds.RSI < 28 {
		confidence := 0.62 // Was 0.66

		if inds.RSI < 20 { // Was 18
			confidence = 0.67 // Was 0.71
		}

		if rsiTurning {
			confidence += 0.04
		}

		signal.Direction = "UP"
		signal.Confidence = confidence
		signal.Reason = "Extreme RSI oversold with reversal"
		return signal
	}

	// ✅ FIXED: Relaxed from 75 to 72
	if inds.RSI > 72 {
		confidence := 0.62 // Was 0.66

		if inds.RSI > 80 { // Was 82
			confidence = 0.67 // Was 0.71
		}

		if rsiTurning {
			confidence += 0.04
		}

		signal.Direction = "DOWN"
		signal.Confidence = confidence
		signal.Reason = "Extreme RSI overbought with reversal"
		return signal
	}

	signal.Direction = "NONE"
	return signal
}

// emaCrossoverSignal - EMA crossover detection
// 🔧 FIXED: Adjusted confidence levels
func (s *VolatilityStrategy) emaCrossoverSignal(ticks []types.Tick, inds types.Indicators) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "EMACrossover",
		Weight: 0.30,
	}

	if len(ticks) < s.config.RSIPeriod+10 {
		signal.Direction = "NONE"
		return signal
	}

	prevInds := indicators.CalculateAllIndicators(ticks[:len(ticks)-5], s.config)

	// Bullish crossover
	if inds.EMA9 > inds.EMA21 && prevInds.EMA9 <= prevInds.EMA21 {
		confidence := 0.66 // Was 0.70

		if inds.RSI < 62 { // Was 60
			confidence += 0.04
		}

		if inds.Momentum > 0.0008 { // Was 0.001
			confidence += 0.03
		}

		signal.Direction = "UP"
		signal.Confidence = confidence
		signal.Reason = "Bullish EMA crossover detected"
		return signal
	}

	// Bearish crossover
	if inds.EMA9 < inds.EMA21 && prevInds.EMA9 >= prevInds.EMA21 {
		confidence := 0.66 // Was 0.70

		if inds.RSI > 38 { // Was 40
			confidence += 0.04
		}

		if inds.Momentum < -0.0008 { // Was -0.001
			confidence += 0.03
		}

		signal.Direction = "DOWN"
		signal.Confidence = confidence
		signal.Reason = "Bearish EMA crossover detected"
		return signal
	}

	signal.Direction = "NONE"
	return signal
}

// IsVolatilityMarket checks if market is a volatility index
func IsVolatilityMarket(market string) bool {
	market = strings.ToLower(market)
	return strings.Contains(market, "volatility") ||
		strings.Contains(market, "v10") ||
		strings.Contains(market, "v25") ||
		strings.Contains(market, "v50") ||
		strings.Contains(market, "v75") ||
		strings.Contains(market, "v100")
}
