package strategy

import (
	"fmt"
	"math"
	"otc-predictor/internal/candles"
	"otc-predictor/internal/indicators"
	"otc-predictor/pkg/types"
	"strings"
	"time"
)

// ForexStrategy for Forex pairs (Rise/Fall contracts)
type ForexStrategy struct {
	config types.StrategyConfig
}

// NewForexStrategy creates a new forex strategy
func NewForexStrategy(config types.StrategyConfig) *ForexStrategy {
	return &ForexStrategy{
		config: config,
	}
}

// AnalyzeWithTimeframe generates signals with timeframe awareness
func (s *ForexStrategy) AnalyzeWithTimeframe(ticks []types.Tick, inds types.Indicators, tfConfig candles.TimeframeConfig, duration int) []types.StrategySignal {
	signals := []types.StrategySignal{}

	if len(ticks) < tfConfig.RSIPeriod*2 {
		return signals
	}

	currentPrice := ticks[len(ticks)-1].Price
	currentTime := ticks[len(ticks)-1].Timestamp

	// Check trading session quality
	sessionMultiplier := s.getSessionMultiplier(currentTime)

	// Check if market conditions are favorable
	if !s.isFavorableCondition(inds, sessionMultiplier) {
		return signals // Return empty if conditions aren't good
	}

	// Strategy 1: Strong Trend Following (MOST RELIABLE)
	trendSignal := s.strongTrendSignal(ticks, inds, sessionMultiplier, duration)
	if trendSignal.Direction != "NONE" {
		signals = append(signals, trendSignal)
	}

	// Strategy 2: EMA Crossover with Multiple Confirmations
	crossSignal := s.confirmedCrossoverSignal(ticks, inds, sessionMultiplier, tfConfig)
	if crossSignal.Direction != "NONE" {
		signals = append(signals, crossSignal)
	}

	// Strategy 3: Support/Resistance Bounces (CONSERVATIVE)
	srSignal := s.conservativeSRSignal(ticks, currentPrice, inds, tfConfig)
	if srSignal.Direction != "NONE" {
		signals = append(signals, srSignal)
	}

	// Strategy 4: Momentum Continuation
	momentumSignal := s.momentumContinuationSignal(ticks, inds, sessionMultiplier)
	if momentumSignal.Direction != "NONE" {
		signals = append(signals, momentumSignal)
	}

	return signals
}

// isFavorableCondition checks if market conditions are good for trading
func (s *ForexStrategy) isFavorableCondition(inds types.Indicators, sessionMult float64) bool {
	// Avoid extreme volatility
	if inds.Volatility > 0.015 {
		return false
	}

	// Need reasonable RSI (not extreme)
	if inds.RSI < 20 || inds.RSI > 80 {
		return false
	}

	// Avoid dead sessions
	if sessionMult < 0.93 {
		return false
	}

	return true
}

// getSessionMultiplier returns confidence multiplier based on trading session
func (s *ForexStrategy) getSessionMultiplier(t time.Time) float64 {
	hour := t.UTC().Hour()

	// Best sessions (London/NY overlap): 12:00-16:00 UTC
	if hour >= 12 && hour < 16 {
		return 1.20 // 20% boost
	}

	// Good sessions (London: 8-12, NY: 13-20 UTC)
	if (hour >= 8 && hour < 12) || (hour >= 13 && hour < 20) {
		return 1.10 // 10% boost
	}

	// Asian session (quieter): 0-7 UTC
	if hour >= 0 && hour < 7 {
		return 0.90 // 10% reduction
	}

	// Off hours
	return 0.85 // 15% reduction
}

// strongTrendSignal - Only signals in VERY strong trends
func (s *ForexStrategy) strongTrendSignal(ticks []types.Tick, inds types.Indicators, sessionMult float64, duration int) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "StrongTrend",
		Weight: 0.45,
	}

	// Calculate trend strength score
	trendScore := 0

	// Check 1: Perfect EMA alignment
	if inds.EMA9 > inds.EMA21 && inds.EMA21 > inds.EMA50 {
		trendScore += 3

		// Check 2: Strong EMA separation (trending)
		emaSpread := (inds.EMA9 - inds.EMA50) / inds.EMA50
		if emaSpread > 0.008 {
			trendScore += 2
		} else if emaSpread > 0.005 {
			trendScore += 1
		}

		// Check 3: RSI confirms uptrend (50-70)
		if inds.RSI > 52 && inds.RSI < 68 {
			trendScore += 2
		}

		// Check 4: Strong positive momentum
		if inds.Momentum > 0.005 {
			trendScore += 2
		} else if inds.Momentum > 0.003 {
			trendScore += 1
		}

		// Check 5: Price above all EMAs and trending
		if inds.BBPosition > 0.2 {
			trendScore += 1
		}

		// Check 6: Consistent upward price action
		recentUpCount := 0
		for i := len(ticks) - 10; i < len(ticks); i++ {
			if i > 0 && ticks[i].Price > ticks[i-1].Price {
				recentUpCount++
			}
		}
		if recentUpCount >= 7 {
			trendScore += 1
		}

		// Only signal if score is very high
		if trendScore >= 8 {
			baseConfidence := 0.65 + float64(trendScore-8)*0.02
			confidence := baseConfidence * sessionMult

			// Duration boost (longer duration = need more confidence)
			if duration >= 900 {
				confidence *= 0.95 // Slight reduction for longer trades
			}

			signal.Direction = "UP"
			signal.Confidence = math.Min(0.88, confidence)
			signal.Reason = fmt.Sprintf("Very strong uptrend (score: %d/12)", trendScore)
			return signal
		}
	}

	// Downtrend check
	trendScore = 0
	if inds.EMA9 < inds.EMA21 && inds.EMA21 < inds.EMA50 {
		trendScore += 3

		emaSpread := (inds.EMA50 - inds.EMA9) / inds.EMA50
		if emaSpread > 0.008 {
			trendScore += 2
		} else if emaSpread > 0.005 {
			trendScore += 1
		}

		if inds.RSI < 48 && inds.RSI > 32 {
			trendScore += 2
		}

		if inds.Momentum < -0.005 {
			trendScore += 2
		} else if inds.Momentum < -0.003 {
			trendScore += 1
		}

		if inds.BBPosition < -0.2 {
			trendScore += 1
		}

		recentDownCount := 0
		for i := len(ticks) - 10; i < len(ticks); i++ {
			if i > 0 && ticks[i].Price < ticks[i-1].Price {
				recentDownCount++
			}
		}
		if recentDownCount >= 7 {
			trendScore += 1
		}

		if trendScore >= 8 {
			baseConfidence := 0.65 + float64(trendScore-8)*0.02
			confidence := baseConfidence * sessionMult

			if duration >= 900 {
				confidence *= 0.95
			}

			signal.Direction = "DOWN"
			signal.Confidence = math.Min(0.88, confidence)
			signal.Reason = fmt.Sprintf("Very strong downtrend (score: %d/12)", trendScore)
			return signal
		}
	}

	signal.Direction = "NONE"
	return signal
}

// confirmedCrossoverSignal - Only signals on fresh crossovers with multiple confirmations
func (s *ForexStrategy) confirmedCrossoverSignal(ticks []types.Tick, inds types.Indicators, sessionMult float64, tfConfig candles.TimeframeConfig) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "ConfirmedCrossover",
		Weight: 0.40,
	}

	lookback := 15
	if len(ticks) < tfConfig.RSIPeriod+lookback {
		signal.Direction = "NONE"
		return signal
	}

	// Get previous indicators
	prevInds := indicators.CalculateAllIndicatorsWithTimeframe(ticks[:len(ticks)-lookback], tfConfig)

	// Bullish crossover
	if inds.EMA9 > inds.EMA21 && prevInds.EMA9 <= prevInds.EMA21 {
		confirmations := 0

		// Confirmation 1: Strong momentum shift
		if inds.Momentum > 0.006 {
			confirmations += 3
		} else if inds.Momentum > 0.004 {
			confirmations += 2
		}

		// Confirmation 2: RSI in buy zone
		if inds.RSI > 48 && inds.RSI < 65 {
			confirmations += 2
		}

		// Confirmation 3: Price above EMA50 (long-term uptrend)
		if inds.EMA21 > inds.EMA50 {
			confirmations += 2
		}

		// Confirmation 4: Volume/price action
		if inds.BBPosition > 0 {
			confirmations += 1
		}

		// Confirmation 5: Not overbought
		if inds.RSI < 70 {
			confirmations += 1
		}

		// Need at least 7 confirmations
		if confirmations >= 7 {
			confidence := (0.62 + float64(confirmations-7)*0.02) * sessionMult
			signal.Direction = "UP"
			signal.Confidence = math.Min(0.85, confidence)
			signal.Reason = fmt.Sprintf("Bullish crossover confirmed (%d/9)", confirmations)
			return signal
		}
	}

	// Bearish crossover
	if inds.EMA9 < inds.EMA21 && prevInds.EMA9 >= prevInds.EMA21 {
		confirmations := 0

		if inds.Momentum < -0.006 {
			confirmations += 3
		} else if inds.Momentum < -0.004 {
			confirmations += 2
		}

		if inds.RSI < 52 && inds.RSI > 35 {
			confirmations += 2
		}

		if inds.EMA21 < inds.EMA50 {
			confirmations += 2
		}

		if inds.BBPosition < 0 {
			confirmations += 1
		}

		if inds.RSI > 30 {
			confirmations += 1
		}

		if confirmations >= 7 {
			confidence := (0.62 + float64(confirmations-7)*0.02) * sessionMult
			signal.Direction = "DOWN"
			signal.Confidence = math.Min(0.85, confidence)
			signal.Reason = fmt.Sprintf("Bearish crossover confirmed (%d/9)", confirmations)
			return signal
		}
	}

	signal.Direction = "NONE"
	return signal
}

// conservativeSRSignal - Very conservative S/R bounces
func (s *ForexStrategy) conservativeSRSignal(ticks []types.Tick, currentPrice float64, inds types.Indicators, tfConfig candles.TimeframeConfig) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "SupportResistance",
		Weight: 0.35,
	}

	levels := s.findKeyLevels(ticks, tfConfig.LookbackPeriod)

	// Support bounce
	for _, level := range levels.Support {
		distance := math.Abs((currentPrice - level) / level)
		if distance < 0.0015 { // Within 0.15%
			strength := 0

			// Multiple touches = stronger level
			touchCount := s.countLevelTouches(ticks, level, 0.002)
			if touchCount >= 3 {
				strength += 3
			} else if touchCount >= 2 {
				strength += 2
			}

			// RSI oversold
			if inds.RSI < 38 {
				strength += 2
			} else if inds.RSI < 45 {
				strength += 1
			}

			// Momentum turning positive
			if inds.Momentum > -0.001 && inds.Momentum < 0.003 {
				strength += 2
			}

			// Price action showing rejection
			if inds.BBPosition < -0.5 {
				strength += 1
			}

			if strength >= 6 {
				confidence := 0.63 + float64(strength-6)*0.02
				signal.Direction = "UP"
				signal.Confidence = math.Min(0.80, confidence)
				signal.Reason = fmt.Sprintf("Strong support bounce (%d touches)", touchCount)
				return signal
			}
		}
	}

	// Resistance rejection
	for _, level := range levels.Resistance {
		distance := math.Abs((currentPrice - level) / level)
		if distance < 0.0015 {
			strength := 0

			touchCount := s.countLevelTouches(ticks, level, 0.002)
			if touchCount >= 3 {
				strength += 3
			} else if touchCount >= 2 {
				strength += 2
			}

			if inds.RSI > 62 {
				strength += 2
			} else if inds.RSI > 55 {
				strength += 1
			}

			if inds.Momentum > -0.003 && inds.Momentum < 0.001 {
				strength += 2
			}

			if inds.BBPosition > 0.5 {
				strength += 1
			}

			if strength >= 6 {
				confidence := 0.63 + float64(strength-6)*0.02
				signal.Direction = "DOWN"
				signal.Confidence = math.Min(0.80, confidence)
				signal.Reason = fmt.Sprintf("Strong resistance rejection (%d touches)", touchCount)
				return signal
			}
		}
	}

	signal.Direction = "NONE"
	return signal
}

// momentumContinuationSignal - Trade with strong momentum
func (s *ForexStrategy) momentumContinuationSignal(ticks []types.Tick, inds types.Indicators, sessionMult float64) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "MomentumContinuation",
		Weight: 0.30,
	}

	// Strong upward momentum
	if inds.Momentum > 0.010 && inds.RSI > 55 && inds.RSI < 75 {
		// Check if trend is still valid
		if inds.EMA9 > inds.EMA21 && inds.TrendStrength > 0.6 {
			confidence := 0.60 * sessionMult

			// Boost for very strong momentum
			if inds.Momentum > 0.015 {
				confidence += 0.05
			}

			signal.Direction = "UP"
			signal.Confidence = math.Min(0.78, confidence)
			signal.Reason = "Strong upward momentum continuation"
			return signal
		}
	}

	// Strong downward momentum
	if inds.Momentum < -0.010 && inds.RSI < 45 && inds.RSI > 25 {
		if inds.EMA9 < inds.EMA21 && inds.TrendStrength > 0.6 {
			confidence := 0.60 * sessionMult

			if inds.Momentum < -0.015 {
				confidence += 0.05
			}

			signal.Direction = "DOWN"
			signal.Confidence = math.Min(0.78, confidence)
			signal.Reason = "Strong downward momentum continuation"
			return signal
		}
	}

	signal.Direction = "NONE"
	return signal
}

// countLevelTouches counts how many times price touched a level
func (s *ForexStrategy) countLevelTouches(ticks []types.Tick, level float64, tolerance float64) int {
	count := 0
	lookback := 100
	if len(ticks) < lookback {
		lookback = len(ticks)
	}

	for i := len(ticks) - lookback; i < len(ticks); i++ {
		distance := math.Abs((ticks[i].Price - level) / level)
		if distance < tolerance {
			count++
		}
	}

	return count
}

// KeyLevels holds support and resistance levels
type KeyLevels struct {
	Support    []float64
	Resistance []float64
}

// findKeyLevels identifies support and resistance from price action
func (s *ForexStrategy) findKeyLevels(ticks []types.Tick, lookback int) KeyLevels {
	levels := KeyLevels{
		Support:    []float64{},
		Resistance: []float64{},
	}

	if len(ticks) < lookback {
		lookback = len(ticks)
	}

	recentTicks := ticks[len(ticks)-lookback:]

	// Find swing highs and lows with larger window
	windowSize := 8
	for i := windowSize; i < len(recentTicks)-windowSize; i++ {
		price := recentTicks[i].Price

		// Check if it's a swing high
		isHigh := true
		for j := i - windowSize; j <= i+windowSize; j++ {
			if j != i && recentTicks[j].Price > price {
				isHigh = false
				break
			}
		}
		if isHigh {
			levels.Resistance = append(levels.Resistance, price)
		}

		// Check if it's a swing low
		isLow := true
		for j := i - windowSize; j <= i+windowSize; j++ {
			if j != i && recentTicks[j].Price < price {
				isLow = false
				break
			}
		}
		if isLow {
			levels.Support = append(levels.Support, price)
		}
	}

	// Keep most recent and strongest levels
	if len(levels.Support) > 5 {
		levels.Support = levels.Support[len(levels.Support)-5:]
	}
	if len(levels.Resistance) > 5 {
		levels.Resistance = levels.Resistance[len(levels.Resistance)-5:]
	}

	return levels
}

// IsForexMarket checks if market is a forex pair
func IsForexMarket(market string) bool {
	market = strings.ToUpper(market)
	forexPairs := []string{"AUD", "EUR", "GBP", "USD", "JPY", "CHF", "CAD", "NZD"}

	for _, currency := range forexPairs {
		if strings.Contains(market, currency) {
			count := 0
			for _, curr := range forexPairs {
				if strings.Contains(market, curr) {
					count++
				}
			}
			if count >= 2 {
				return true
			}
		}
	}

	return false
}
