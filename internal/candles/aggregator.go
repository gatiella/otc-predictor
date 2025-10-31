package candles

import (
	"otc-predictor/pkg/types"
	"time"
)

// TimeframeConfig holds timeframe-specific settings
type TimeframeConfig struct {
	CandlePeriod   time.Duration // 1min, 5min, 15min
	MinCandles     int           // Minimum candles needed
	RSIPeriod      int           // Dynamic RSI period
	EMAFast        int           // Dynamic EMA fast
	EMASlow        int           // Dynamic EMA slow
	EMATrend       int           // Dynamic EMA trend
	LookbackPeriod int           // For S/R levels
}

// GetTimeframeConfig returns optimal settings for a duration
// ✅ ULTRA-FAST: Dramatically reduced MinCandles for faster signals
func GetTimeframeConfig(durationSeconds int, marketType string) TimeframeConfig {
	if marketType == "forex" {
		// FOREX: Slower ticking, longer timeframes
		switch {
		case durationSeconds <= 900: // 15 minutes
			return TimeframeConfig{
				CandlePeriod:   1 * time.Minute, // 1-min candles
				MinCandles:     15,              // ✅ Was 60! Now just 15 min history
				RSIPeriod:      14,
				EMAFast:        9,
				EMASlow:        21,
				EMATrend:       50,
				LookbackPeriod: 50, // Was 100
			}
		case durationSeconds <= 1800: // 30 minutes
			return TimeframeConfig{
				CandlePeriod:   2 * time.Minute, // 2-min candles
				MinCandles:     18,              // ✅ Was 60! Now 36 min history
				RSIPeriod:      14,
				EMAFast:        9,
				EMASlow:        21,
				EMATrend:       50,
				LookbackPeriod: 60, // Was 150
			}
		default: // 1 hour+
			return TimeframeConfig{
				CandlePeriod:   5 * time.Minute, // 5-min candles
				MinCandles:     20,              // ✅ Was 60! Now 100 min history
				RSIPeriod:      14,
				EMAFast:        9,
				EMASlow:        21,
				EMATrend:       50,
				LookbackPeriod: 80, // Was 200
			}
		}
	}

	// SYNTHETICS: Fast ticking, shorter timeframes
	switch {
	case durationSeconds <= 60: // 1 minute
		return TimeframeConfig{
			CandlePeriod:   5 * time.Second, // 5-sec candles
			MinCandles:     15,              // ✅ Was 50! Now ~75 sec history
			RSIPeriod:      14,
			EMAFast:        9,
			EMASlow:        21,
			EMATrend:       50,
			LookbackPeriod: 40, // Was 80
		}
	case durationSeconds <= 180: // 3 minutes
		return TimeframeConfig{
			CandlePeriod:   10 * time.Second, // 10-sec candles
			MinCandles:     20,               // ✅ Was 50! Now ~3.3 min history
			RSIPeriod:      14,
			EMAFast:        9,
			EMASlow:        21,
			EMATrend:       50,
			LookbackPeriod: 50, // Was 100
		}
	default: // 5 minutes+
		return TimeframeConfig{
			CandlePeriod:   30 * time.Second, // 30-sec candles
			MinCandles:     25,               // ✅ Was 50! Now ~12.5 min history
			RSIPeriod:      14,
			EMAFast:        9,
			EMASlow:        21,
			EMATrend:       50,
			LookbackPeriod: 60, // Was 120
		}
	}
}

// TicksToCandles converts ticks to OHLC candles
func TicksToCandles(ticks []types.Tick, period time.Duration) []types.Candle {
	if len(ticks) == 0 {
		return []types.Candle{}
	}

	candles := []types.Candle{}
	var currentCandle *types.Candle

	for _, tick := range ticks {
		// Align timestamp to candle period
		candleTime := tick.Timestamp.Truncate(period)

		// Start new candle or continue existing
		if currentCandle == nil || !currentCandle.Timestamp.Equal(candleTime) {
			// Save previous candle
			if currentCandle != nil {
				candles = append(candles, *currentCandle)
			}

			// Start new candle
			currentCandle = &types.Candle{
				Market:    tick.Market,
				Open:      tick.Price,
				High:      tick.Price,
				Low:       tick.Price,
				Close:     tick.Price,
				Volume:    1,
				Timestamp: candleTime,
			}
		} else {
			// Update existing candle
			if tick.Price > currentCandle.High {
				currentCandle.High = tick.Price
			}
			if tick.Price < currentCandle.Low {
				currentCandle.Low = tick.Price
			}
			currentCandle.Close = tick.Price
			currentCandle.Volume++
		}
	}

	// Add last candle
	if currentCandle != nil {
		candles = append(candles, *currentCandle)
	}

	return candles
}

// CandlesToTicks converts candles back to tick format for indicator calculations
func CandlesToTicks(candles []types.Candle) []types.Tick {
	ticks := make([]types.Tick, len(candles))

	for i, candle := range candles {
		ticks[i] = types.Tick{
			Market:    candle.Market,
			Price:     candle.Close,
			Timestamp: candle.Timestamp,
			Epoch:     candle.Timestamp.Unix(),
		}
	}

	return ticks
}

// GetMinimumTicksRequired calculates minimum ticks needed for a timeframe
// ✅ ULTRA-FAST: Absolute minimum for quick signals
func GetMinimumTicksRequired(config TimeframeConfig, marketType string) int {
	// ✅ FIXED: Direct calculation without buffers
	if marketType == "forex" {
		// Forex: ~2 ticks per minute average
		// For 15 min candles, we need just 15 ticks (one per candle)
		// Add small safety margin
		return config.MinCandles + 5 // Was: complex calculation with 1.5x buffer
	}

	// Synthetics: Fast ticking
	// For 5-sec candles, we need ~15 candles = 75 seconds = 75 ticks
	return config.MinCandles + 10 // Was: complex calculation with 1.5x buffer
}

// ValidateCandles checks if we have enough quality candles
// ✅ ULTRA-RELAXED: Only block truly bad data
func ValidateCandles(candles []types.Candle, minRequired int) (bool, string) {
	// ✅ RELAXED: Accept 80% of minimum (was 100%)
	if len(candles) < int(float64(minRequired)*0.8) {
		return false, "insufficient_candles"
	}

	// ✅ REMOVED: Gap detection (too strict for forex)
	// Forex can have natural gaps during slow periods

	// ✅ RELAXED: Allow up to 50% zero-volume (was 30%)
	// Some candles might not have multiple ticks
	zeroVolCount := 0
	for _, c := range candles {
		if c.Volume == 0 {
			zeroVolCount++
		}
	}

	if len(candles) > 0 && float64(zeroVolCount)/float64(len(candles)) > 0.5 {
		return false, "stale_data"
	}

	return true, "valid"
}
