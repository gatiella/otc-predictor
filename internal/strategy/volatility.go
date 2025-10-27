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

	// Strategy 5: EMA Crossover (NEW - strong signal)
	crossoverSignal := s.emaCrossoverSignal(ticks, inds)
	if crossoverSignal.Direction != "NONE" {
		signals = append(signals, crossoverSignal)
	}

	return signals
}

// meanReversionSignal - Price returns to mean (HIGHEST WEIGHT)
func (s *VolatilityStrategy) meanReversionSignal(price float64, inds types.Indicators, pattern string) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "MeanReversion",
		Weight: 0.45, // Increased from 0.4
	}

	// STRICT CONDITIONS: Oversold at lower band + momentum turning
	if inds.RSI < s.config.RSIOversold && inds.BBPosition < -0.75 {
		confidence := 0.72 // Increased base confidence

		// Pattern confirmation
		if pattern == "double_bottom" {
			confidence = 0.78
		}

		// Extremely oversold
		if inds.RSI < 22 && inds.BBPosition < -0.85 {
			confidence += 0.06
		}

		// Momentum turning positive
		if inds.Momentum > -0.0005 {
			confidence += 0.03
		}

		signal.Direction = "UP"
		signal.Confidence = confidence
		signal.Reason = "Strong oversold mean reversion signal"
		return signal
	}

	// STRICT CONDITIONS: Overbought at upper band + momentum turning
	if inds.RSI > s.config.RSIOverbought && inds.BBPosition > 0.75 {
		confidence := 0.72

		// Pattern confirmation
		if pattern == "double_top" {
			confidence = 0.78
		}

		// Extremely overbought
		if inds.RSI > 78 && inds.BBPosition > 0.85 {
			confidence += 0.06
		}

		// Momentum turning negative
		if inds.Momentum < 0.0005 {
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

// momentumSignal - Clear trend following (STRICTER)
func (s *VolatilityStrategy) momentumSignal(inds types.Indicators) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "Momentum",
		Weight: 0.35, // Increased from 0.3
	}

	// Bullish: Strong EMA alignment + not overbought + momentum
	if inds.EMA9 > inds.EMA21 && inds.EMA21 > inds.EMA50 &&
		inds.RSI < 62 && inds.RSI > 48 && inds.Momentum > 0.002 {

		confidence := 0.68

		// Strong trend confirmation
		if inds.TrendStrength > 0.75 {
			confidence = 0.73
		}

		// Price above all EMAs
		emaDistance := (inds.EMA9 - inds.EMA50) / inds.EMA50
		if emaDistance > 0.003 {
			confidence += 0.04
		}

		signal.Direction = "UP"
		signal.Confidence = confidence
		signal.Reason = "Strong bullish momentum alignment"
		return signal
	}

	// Bearish: Strong EMA alignment + not oversold + momentum
	if inds.EMA9 < inds.EMA21 && inds.EMA21 < inds.EMA50 &&
		inds.RSI > 38 && inds.RSI < 52 && inds.Momentum < -0.002 {

		confidence := 0.68

		// Strong trend confirmation
		if inds.TrendStrength > 0.75 {
			confidence = 0.73
		}

		// Price below all EMAs
		emaDistance := (inds.EMA50 - inds.EMA9) / inds.EMA50
		if emaDistance > 0.003 {
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
func (s *VolatilityStrategy) bollingerBandSignal(price float64, inds types.Indicators) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "BollingerBands",
		Weight: 0.25, // Increased from 0.2
	}

	// At lower band - expect bounce
	if inds.BBPosition < -0.85 && price < inds.BBLower {
		confidence := 0.69

		// RSI confirms oversold
		if inds.RSI < 35 {
			confidence += 0.05
		}

		signal.Direction = "UP"
		signal.Confidence = confidence
		signal.Reason = "Price at lower BB band - bounce expected"
		return signal
	}

	// At upper band - expect pullback
	if inds.BBPosition > 0.85 && price > inds.BBUpper {
		confidence := 0.69

		// RSI confirms overbought
		if inds.RSI > 65 {
			confidence += 0.05
		}

		signal.Direction = "DOWN"
		signal.Confidence = confidence
		signal.Reason = "Price at upper BB band - pullback expected"
		return signal
	}

	// BB Squeeze breakout (lower priority)
	bbWidth := (inds.BBUpper - inds.BBLower) / inds.BBMiddle
	if bbWidth < 0.015 {
		if price > inds.BBMiddle && inds.Momentum > 0.003 {
			signal.Direction = "UP"
			signal.Confidence = 0.66
			signal.Reason = "BB squeeze breakout upward"
			return signal
		}

		if price < inds.BBMiddle && inds.Momentum < -0.003 {
			signal.Direction = "DOWN"
			signal.Confidence = 0.66
			signal.Reason = "BB squeeze breakout downward"
			return signal
		}
	}

	signal.Direction = "NONE"
	return signal
}

// rsiSignal - RSI at true extremes with confirmation
func (s *VolatilityStrategy) rsiSignal(inds types.Indicators, ticks []types.Tick) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "RSI",
		Weight: 0.15, // Increased from 0.1
	}

	// Check RSI trend (is it turning?)
	rsiTurning := false
	if len(ticks) > s.config.RSIPeriod+5 {
		prevInds := indicators.CalculateAllIndicators(ticks[:len(ticks)-5], s.config)
		if inds.RSI < 25 && prevInds.RSI < inds.RSI {
			rsiTurning = true // RSI was falling, now rising
		}
		if inds.RSI > 75 && prevInds.RSI > inds.RSI {
			rsiTurning = true // RSI was rising, now falling
		}
	}

	// Extreme oversold with confirmation
	if inds.RSI < 25 {
		confidence := 0.66

		if inds.RSI < 18 {
			confidence = 0.71 // Very extreme
		}

		if rsiTurning {
			confidence += 0.04 // RSI turning around
		}

		signal.Direction = "UP"
		signal.Confidence = confidence
		signal.Reason = "Extreme RSI oversold with reversal"
		return signal
	}

	// Extreme overbought with confirmation
	if inds.RSI > 75 {
		confidence := 0.66

		if inds.RSI > 82 {
			confidence = 0.71 // Very extreme
		}

		if rsiTurning {
			confidence += 0.04 // RSI turning around
		}

		signal.Direction = "DOWN"
		signal.Confidence = confidence
		signal.Reason = "Extreme RSI overbought with reversal"
		return signal
	}

	signal.Direction = "NONE"
	return signal
}

// emaCrossoverSignal - NEW: EMA crossover detection
func (s *VolatilityStrategy) emaCrossoverSignal(ticks []types.Tick, inds types.Indicators) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "EMACrossover",
		Weight: 0.30,
	}

	// Need enough data to check previous state
	if len(ticks) < s.config.RSIPeriod+10 {
		signal.Direction = "NONE"
		return signal
	}

	// Get indicators from 5 ticks ago
	prevInds := indicators.CalculateAllIndicators(ticks[:len(ticks)-5], s.config)

	// Bullish crossover: EMA9 crosses above EMA21
	if inds.EMA9 > inds.EMA21 && prevInds.EMA9 <= prevInds.EMA21 {
		confidence := 0.70

		// Stronger if RSI not overbought
		if inds.RSI < 60 {
			confidence += 0.04
		}

		// Stronger if momentum confirms
		if inds.Momentum > 0.001 {
			confidence += 0.03
		}

		signal.Direction = "UP"
		signal.Confidence = confidence
		signal.Reason = "Bullish EMA crossover detected"
		return signal
	}

	// Bearish crossover: EMA9 crosses below EMA21
	if inds.EMA9 < inds.EMA21 && prevInds.EMA9 >= prevInds.EMA21 {
		confidence := 0.70

		// Stronger if RSI not oversold
		if inds.RSI > 40 {
			confidence += 0.04
		}

		// Stronger if momentum confirms
		if inds.Momentum < -0.001 {
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
