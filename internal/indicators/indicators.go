package indicators

import (
	"math"
	"otc-predictor/pkg/types"
)

// CalculateRSI calculates Relative Strength Index
func CalculateRSI(ticks []types.Tick, period int) float64 {
	if len(ticks) < period+1 {
		return 50.0 // Neutral
	}

	gains := 0.0
	losses := 0.0

	// Calculate initial average gain/loss
	for i := len(ticks) - period; i < len(ticks); i++ {
		change := ticks[i].Price - ticks[i-1].Price
		if change > 0 {
			gains += change
		} else {
			losses += math.Abs(change)
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	if avgLoss == 0 {
		return 100.0
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))

	return rsi
}

// CalculateEMA calculates Exponential Moving Average
func CalculateEMA(ticks []types.Tick, period int) float64 {
	if len(ticks) < period {
		return ticks[len(ticks)-1].Price
	}

	multiplier := 2.0 / float64(period+1)

	// Start with SMA
	sum := 0.0
	for i := len(ticks) - period; i < len(ticks); i++ {
		sum += ticks[i].Price
	}
	ema := sum / float64(period)

	// Calculate EMA
	for i := len(ticks) - period + 1; i < len(ticks); i++ {
		ema = (ticks[i].Price-ema)*multiplier + ema
	}

	return ema
}

// CalculateSMA calculates Simple Moving Average
func CalculateSMA(ticks []types.Tick, period int) float64 {
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
func CalculateBollingerBands(ticks []types.Tick, period int, stdDev float64) BollingerBands {
	if len(ticks) < period {
		currentPrice := ticks[len(ticks)-1].Price
		return BollingerBands{
			Upper:  currentPrice * 1.02,
			Middle: currentPrice,
			Lower:  currentPrice * 0.98,
		}
	}

	// Calculate middle band (SMA)
	middle := CalculateSMA(ticks, period)

	// Calculate standard deviation
	sum := 0.0
	for i := len(ticks) - period; i < len(ticks); i++ {
		diff := ticks[i].Price - middle
		sum += diff * diff
	}
	stdDeviation := math.Sqrt(sum / float64(period))

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
	if len(ticks) < period {
		period = len(ticks)
	}
	if period < 2 {
		return 0
	}

	// Calculate mean
	mean := 0.0
	for i := len(ticks) - period; i < len(ticks); i++ {
		mean += ticks[i].Price
	}
	mean /= float64(period)

	// Calculate variance
	variance := 0.0
	for i := len(ticks) - period; i < len(ticks); i++ {
		diff := ticks[i].Price - mean
		variance += diff * diff
	}
	variance /= float64(period)

	return math.Sqrt(variance)
}

// CalculateMomentum calculates price momentum
func CalculateMomentum(ticks []types.Tick, period int) float64 {
	if len(ticks) < period+1 {
		return 0
	}

	currentPrice := ticks[len(ticks)-1].Price
	pastPrice := ticks[len(ticks)-period-1].Price

	if pastPrice == 0 {
		return 0
	}

	return ((currentPrice - pastPrice) / pastPrice) * 100
}

// CalculateTrendStrength calculates trend strength using ADX concept
func CalculateTrendStrength(ticks []types.Tick, period int) float64 {
	if len(ticks) < period+1 {
		return 0
	}

	ema9 := CalculateEMA(ticks, 9)
	ema21 := CalculateEMA(ticks, 21)
	ema50 := CalculateEMA(ticks, 50)

	currentPrice := ticks[len(ticks)-1].Price

	// All EMAs aligned = strong trend
	if (ema9 > ema21 && ema21 > ema50) || (ema9 < ema21 && ema21 < ema50) {
		// Calculate strength based on separation
		separation := math.Abs(ema9-ema50) / currentPrice
		return math.Min(separation*100, 1.0)
	}

	return 0.3 // Weak or no trend
}

// CalculateAllIndicators calculates all indicators at once
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
		TrendStrength: CalculateTrendStrength(ticks, 50),
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
