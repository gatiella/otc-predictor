package indicators

import (
	"math"
	"otc-predictor/internal/candles"
	"otc-predictor/pkg/types"
)

// CalculateRSI calculates Relative Strength Index
// ✅ RELAXED: Works with minimal data
func CalculateRSI(ticks []types.Tick, period int) float64 {
	// ✅ RELAXED: Accept less data (was period+1, now period/2)
	if len(ticks) < period/2 {
		return 50.0 // Neutral
	}

	// Use available data if we don't have full period
	actualPeriod := period
	if len(ticks)-1 < period {
		actualPeriod = len(ticks) - 1
	}
	if actualPeriod < 5 {
		actualPeriod = 5 // Minimum useful RSI
	}

	gains := 0.0
	losses := 0.0

	// Calculate initial average gain/loss
	startIdx := len(ticks) - actualPeriod
	if startIdx < 1 {
		startIdx = 1
	}

	for i := startIdx; i < len(ticks); i++ {
		change := ticks[i].Price - ticks[i-1].Price
		if change > 0 {
			gains += change
		} else {
			losses += math.Abs(change)
		}
	}

	avgGain := gains / float64(actualPeriod)
	avgLoss := losses / float64(actualPeriod)

	if avgLoss == 0 {
		if avgGain > 0 {
			return 100.0
		}
		return 50.0
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))

	return rsi
}

// CalculateEMA calculates Exponential Moving Average
// ✅ RELAXED: Works with partial data
func CalculateEMA(ticks []types.Tick, period int) float64 {
	if len(ticks) == 0 {
		return 0
	}

	// ✅ RELAXED: Use what we have (was required full period)
	if len(ticks) < period/3 {
		// Not enough data, return simple average
		sum := 0.0
		for _, tick := range ticks {
			sum += tick.Price
		}
		return sum / float64(len(ticks))
	}

	actualPeriod := period
	if len(ticks) < period {
		actualPeriod = len(ticks)
	}

	multiplier := 2.0 / float64(actualPeriod+1)

	// Start with SMA
	sum := 0.0
	startIdx := len(ticks) - actualPeriod
	for i := startIdx; i < len(ticks); i++ {
		sum += ticks[i].Price
	}
	ema := sum / float64(actualPeriod)

	// Calculate EMA
	for i := startIdx + 1; i < len(ticks); i++ {
		ema = (ticks[i].Price-ema)*multiplier + ema
	}

	return ema
}

// CalculateSMA calculates Simple Moving Average
func CalculateSMA(ticks []types.Tick, period int) float64 {
	if len(ticks) == 0 {
		return 0
	}

	if len(ticks) < period {
		period = len(ticks)
	}

	sum := 0.0
	for i := len(ticks) - period; i < len(ticks); i++ {
		sum += ticks[i].Price
	}

	return sum / float64(period)
}

// BollingerBands holds BB values
type BollingerBands struct {
	Upper  float64
	Middle float64
	Lower  float64
}

// CalculateBollingerBands calculates Bollinger Bands
// ✅ RELAXED: Works with minimal data
func CalculateBollingerBands(ticks []types.Tick, period int, stdDev float64) BollingerBands {
	if len(ticks) == 0 {
		return BollingerBands{Upper: 0, Middle: 0, Lower: 0}
	}

	// ✅ RELAXED: Use less data if needed (was required full period)
	actualPeriod := period
	if len(ticks) < period {
		actualPeriod = len(ticks)
		if actualPeriod < 5 {
			// Not enough data - return wide bands
			currentPrice := ticks[len(ticks)-1].Price
			return BollingerBands{
				Upper:  currentPrice * 1.03,
				Middle: currentPrice,
				Lower:  currentPrice * 0.97,
			}
		}
	}

	// Calculate middle band (SMA)
	middle := CalculateSMA(ticks, actualPeriod)

	// Calculate standard deviation
	sum := 0.0
	for i := len(ticks) - actualPeriod; i < len(ticks); i++ {
		diff := ticks[i].Price - middle
		sum += diff * diff
	}
	stdDeviation := math.Sqrt(sum / float64(actualPeriod))

	return BollingerBands{
		Upper:  middle + (stdDeviation * stdDev),
		Middle: middle,
		Lower:  middle - (stdDeviation * stdDev),
	}
}

// CalculateBBPosition calculates position relative to BB (-1 to 1)
func CalculateBBPosition(price float64, bb BollingerBands) float64 {
	if bb.Upper == bb.Lower {
		return 0
	}

	// Normalize to -1 (at lower) to 1 (at upper)
	position := (price - bb.Middle) / (bb.Upper - bb.Middle)

	// Clamp to -1 to 1
	if position > 1 {
		position = 1
	} else if position < -1 {
		position = -1
	}

	return position
}

// CalculateVolatility calculates price volatility (standard deviation)
func CalculateVolatility(ticks []types.Tick, period int) float64 {
	if len(ticks) < 2 {
		return 0
	}

	actualPeriod := period
	if len(ticks) < period {
		actualPeriod = len(ticks)
	}

	// Calculate mean
	mean := 0.0
	for i := len(ticks) - actualPeriod; i < len(ticks); i++ {
		mean += ticks[i].Price
	}
	mean /= float64(actualPeriod)

	// Calculate variance
	variance := 0.0
	for i := len(ticks) - actualPeriod; i < len(ticks); i++ {
		diff := ticks[i].Price - mean
		variance += diff * diff
	}
	variance /= float64(actualPeriod)

	return math.Sqrt(variance)
}

// CalculateMomentum calculates price momentum
// ✅ RELAXED: Works with less data
func CalculateMomentum(ticks []types.Tick, period int) float64 {
	if len(ticks) < 2 {
		return 0
	}

	// ✅ RELAXED: Use available data
	actualPeriod := period
	if len(ticks)-1 < period {
		actualPeriod = len(ticks) - 1
	}
	if actualPeriod < 1 {
		actualPeriod = 1
	}

	currentPrice := ticks[len(ticks)-1].Price
	pastPrice := ticks[len(ticks)-actualPeriod-1].Price

	if pastPrice == 0 {
		return 0
	}

	return ((currentPrice - pastPrice) / pastPrice) * 100
}

// CalculateTrendStrength calculates trend strength using ADX concept
// ✅ RELAXED: Works with partial EMA data
func CalculateTrendStrength(ticks []types.Tick, emaFast, emaSlow, emaTrend int) float64 {
	if len(ticks) < 5 {
		return 0
	}

	emaF := CalculateEMA(ticks, emaFast)
	emaS := CalculateEMA(ticks, emaSlow)
	emaT := CalculateEMA(ticks, emaTrend)

	if emaF == 0 || emaS == 0 || emaT == 0 {
		return 0
	}

	currentPrice := ticks[len(ticks)-1].Price

	// All EMAs aligned = strong trend
	if (emaF > emaS && emaS > emaT) || (emaF < emaS && emaS < emaT) {
		// Calculate strength based on separation
		separation := math.Abs(emaF-emaT) / currentPrice
		return math.Min(separation*100, 1.0)
	}

	return 0.3 // Weak or no trend
}

// CalculateAllIndicators calculates all indicators with custom config
func CalculateAllIndicators(ticks []types.Tick, config types.StrategyConfig) types.Indicators {
	if len(ticks) == 0 {
		return types.Indicators{}
	}

	currentPrice := ticks[len(ticks)-1].Price
	bb := CalculateBollingerBands(ticks, config.BBPeriod, config.BBStdDev)

	return types.Indicators{
		RSI:           CalculateRSI(ticks, config.RSIPeriod),
		EMA9:          CalculateEMA(ticks, config.EMAFast),
		EMA21:         CalculateEMA(ticks, config.EMASlow),
		EMA50:         CalculateEMA(ticks, config.EMATrend),
		BBUpper:       bb.Upper,
		BBMiddle:      bb.Middle,
		BBLower:       bb.Lower,
		BBPosition:    CalculateBBPosition(currentPrice, bb),
		Volatility:    CalculateVolatility(ticks, 20),
		Momentum:      CalculateMomentum(ticks, 10),
		TrendStrength: CalculateTrendStrength(ticks, config.EMAFast, config.EMASlow, config.EMATrend),
	}
}

// CalculateAllIndicatorsWithTimeframe calculates indicators with timeframe-aware config
func CalculateAllIndicatorsWithTimeframe(ticks []types.Tick, tfConfig candles.TimeframeConfig) types.Indicators {
	if len(ticks) == 0 {
		return types.Indicators{}
	}

	currentPrice := ticks[len(ticks)-1].Price
	bb := CalculateBollingerBands(ticks, 20, 2.0)

	return types.Indicators{
		RSI:           CalculateRSI(ticks, tfConfig.RSIPeriod),
		EMA9:          CalculateEMA(ticks, tfConfig.EMAFast),
		EMA21:         CalculateEMA(ticks, tfConfig.EMASlow),
		EMA50:         CalculateEMA(ticks, tfConfig.EMATrend),
		BBUpper:       bb.Upper,
		BBMiddle:      bb.Middle,
		BBLower:       bb.Lower,
		BBPosition:    CalculateBBPosition(currentPrice, bb),
		Volatility:    CalculateVolatility(ticks, 20),
		Momentum:      CalculateMomentum(ticks, tfConfig.RSIPeriod),
		TrendStrength: CalculateTrendStrength(ticks, tfConfig.EMAFast, tfConfig.EMASlow, tfConfig.EMATrend),
	}
}

// DetectPattern detects chart patterns
func DetectPattern(ticks []types.Tick) string {
	if len(ticks) < 5 {
		return "insufficient_data"
	}

	recent := ticks[len(ticks)-5:]

	// Double bottom
	if recent[0].Price < recent[1].Price &&
		recent[1].Price > recent[2].Price &&
		recent[2].Price < recent[3].Price &&
		recent[3].Price > recent[4].Price &&
		math.Abs(recent[0].Price-recent[2].Price) < recent[0].Price*0.001 {
		return "double_bottom"
	}

	// Double top
	if recent[0].Price > recent[1].Price &&
		recent[1].Price < recent[2].Price &&
		recent[2].Price > recent[3].Price &&
		recent[3].Price < recent[4].Price &&
		math.Abs(recent[0].Price-recent[2].Price) < recent[0].Price*0.001 {
		return "double_top"
	}

	// Strong uptrend
	upCount := 0
	for i := 1; i < len(recent); i++ {
		if recent[i].Price > recent[i-1].Price {
			upCount++
		}
	}
	if upCount >= 4 {
		return "strong_uptrend"
	}

	// Strong downtrend
	if upCount == 0 {
		return "strong_downtrend"
	}

	return "consolidation"
}
