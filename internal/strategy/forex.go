package strategy

import (
	"math"
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

// Analyze generates signals for forex pairs
func (s *ForexStrategy) Analyze(ticks []types.Tick, inds types.Indicators) []types.StrategySignal {
	signals := []types.StrategySignal{}

	if len(ticks) < s.config.RSIPeriod*2 {
		return signals
	}

	currentPrice := ticks[len(ticks)-1].Price
	currentTime := ticks[len(ticks)-1].Timestamp

	// Check trading session quality
	sessionMultiplier := s.getSessionMultiplier(currentTime)

	// Strategy 1: Trend Following (STRONGEST for forex)
	trendSignal := s.trendFollowingSignal(ticks, inds, sessionMultiplier)
	if trendSignal.Direction != "NONE" {
		signals = append(signals, trendSignal)
	}

	// Strategy 2: Support/Resistance Levels
	srSignal := s.supportResistanceSignal(ticks, currentPrice, inds)
	if srSignal.Direction != "NONE" {
		signals = append(signals, srSignal)
	}

	// Strategy 3: EMA Crossover with Momentum
	crossSignal := s.emaCrossoverMomentumSignal(ticks, inds, sessionMultiplier)
	if crossSignal.Direction != "NONE" {
		signals = append(signals, crossSignal)
	}

	// Strategy 4: Pullback Trading (best during trends)
	pullbackSignal := s.pullbackSignal(ticks, inds, sessionMultiplier)
	if pullbackSignal.Direction != "NONE" {
		signals = append(signals, pullbackSignal)
	}

	// Strategy 5: Range Trading (consolidation periods)
	rangeSignal := s.rangeSignal(currentPrice, inds)
	if rangeSignal.Direction != "NONE" {
		signals = append(signals, rangeSignal)
	}

	return signals
}

// getSessionMultiplier returns confidence multiplier based on trading session
func (s *ForexStrategy) getSessionMultiplier(t time.Time) float64 {
	hour := t.UTC().Hour()

	// Best sessions (London/NY overlap): 12:00-16:00 UTC
	if hour >= 12 && hour < 16 {
		return 1.15 // 15% boost
	}

	// Good sessions (London: 8-12, NY: 13-17 UTC)
	if (hour >= 8 && hour < 12) || (hour >= 13 && hour < 17) {
		return 1.08 // 8% boost
	}

	// Asian session (quieter): 0-7 UTC
	if hour >= 0 && hour < 7 {
		return 0.95 // 5% reduction
	}

	// Off hours (17-24 UTC)
	return 0.92 // 8% reduction
}

// trendFollowingSignal - Follow strong trends (HIGHEST WEIGHT)
func (s *ForexStrategy) trendFollowingSignal(ticks []types.Tick, inds types.Indicators, sessionMult float64) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "TrendFollowing",
		Weight: 0.40,
	}

	// Strong uptrend: EMA alignment + RSI in range + momentum
	if inds.EMA9 > inds.EMA21 && inds.EMA21 > inds.EMA50 &&
		inds.RSI > 50 && inds.RSI < 70 && inds.Momentum > 0.003 {

		confidence := 0.72

		// Very strong trend
		emaSpread := (inds.EMA9 - inds.EMA50) / inds.EMA50
		if emaSpread > 0.005 {
			confidence += 0.05
		}

		// Price above all EMAs
		if inds.BBPosition > 0.3 {
			confidence += 0.03
		}

		// Apply session multiplier
		confidence *= sessionMult

		signal.Direction = "UP"
		signal.Confidence = math.Min(0.82, confidence)
		signal.Reason = "Strong uptrend with momentum"
		return signal
	}

	// Strong downtrend: EMA alignment + RSI in range + momentum
	if inds.EMA9 < inds.EMA21 && inds.EMA21 < inds.EMA50 &&
		inds.RSI < 50 && inds.RSI > 30 && inds.Momentum < -0.003 {

		confidence := 0.72

		// Very strong trend
		emaSpread := (inds.EMA50 - inds.EMA9) / inds.EMA50
		if emaSpread > 0.005 {
			confidence += 0.05
		}

		// Price below all EMAs
		if inds.BBPosition < -0.3 {
			confidence += 0.03
		}

		// Apply session multiplier
		confidence *= sessionMult

		signal.Direction = "DOWN"
		signal.Confidence = math.Min(0.82, confidence)
		signal.Reason = "Strong downtrend with momentum"
		return signal
	}

	signal.Direction = "NONE"
	return signal
}

// supportResistanceSignal - Bounce from S/R levels
func (s *ForexStrategy) supportResistanceSignal(ticks []types.Tick, currentPrice float64, inds types.Indicators) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "SupportResistance",
		Weight: 0.35,
	}

	// Calculate key levels from recent price action
	levels := s.findKeyLevels(ticks, 100)

	// Check if price is near a support level
	for _, level := range levels.Support {
		distance := math.Abs((currentPrice - level) / level)
		if distance < 0.002 { // Within 0.2% of support
			confidence := 0.70

			// RSI oversold confirms
			if inds.RSI < 40 {
				confidence += 0.06
			}

			// Price bouncing (momentum turning positive)
			if inds.Momentum > -0.001 && inds.Momentum < 0.001 {
				confidence += 0.04
			}

			signal.Direction = "UP"
			signal.Confidence = confidence
			signal.Reason = "Bounce from support level"
			return signal
		}
	}

	// Check if price is near a resistance level
	for _, level := range levels.Resistance {
		distance := math.Abs((currentPrice - level) / level)
		if distance < 0.002 { // Within 0.2% of resistance
			confidence := 0.70

			// RSI overbought confirms
			if inds.RSI > 60 {
				confidence += 0.06
			}

			// Price rejecting (momentum turning negative)
			if inds.Momentum > -0.001 && inds.Momentum < 0.001 {
				confidence += 0.04
			}

			signal.Direction = "DOWN"
			signal.Confidence = confidence
			signal.Reason = "Rejection from resistance level"
			return signal
		}
	}

	signal.Direction = "NONE"
	return signal
}

// emaCrossoverMomentumSignal - EMA crossovers with strong momentum
func (s *ForexStrategy) emaCrossoverMomentumSignal(ticks []types.Tick, inds types.Indicators, sessionMult float64) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "EMACrossover",
		Weight: 0.35,
	}

	if len(ticks) < s.config.RSIPeriod+15 {
		signal.Direction = "NONE"
		return signal
	}

	// Get previous indicators
	prevInds := indicators.CalculateAllIndicators(ticks[:len(ticks)-8], s.config)

	// Bullish crossover: EMA9 just crossed above EMA21
	if inds.EMA9 > inds.EMA21 && prevInds.EMA9 <= prevInds.EMA21 {
		confidence := 0.73

		// Strong momentum confirms
		if inds.Momentum > 0.004 {
			confidence += 0.05
		}

		// RSI in good zone
		if inds.RSI > 45 && inds.RSI < 65 {
			confidence += 0.04
		}

		// Apply session multiplier
		confidence *= sessionMult

		signal.Direction = "UP"
		signal.Confidence = math.Min(0.82, confidence)
		signal.Reason = "Bullish EMA crossover with momentum"
		return signal
	}

	// Bearish crossover: EMA9 just crossed below EMA21
	if inds.EMA9 < inds.EMA21 && prevInds.EMA9 >= prevInds.EMA21 {
		confidence := 0.73

		// Strong momentum confirms
		if inds.Momentum < -0.004 {
			confidence += 0.05
		}

		// RSI in good zone
		if inds.RSI > 35 && inds.RSI < 55 {
			confidence += 0.04
		}

		// Apply session multiplier
		confidence *= sessionMult

		signal.Direction = "DOWN"
		signal.Confidence = math.Min(0.82, confidence)
		signal.Reason = "Bearish EMA crossover with momentum"
		return signal
	}

	signal.Direction = "NONE"
	return signal
}

// pullbackSignal - Trade pullbacks in strong trends
func (s *ForexStrategy) pullbackSignal(ticks []types.Tick, inds types.Indicators, sessionMult float64) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "Pullback",
		Weight: 0.30,
	}

	// Uptrend with pullback
	if inds.EMA9 > inds.EMA21 && inds.EMA21 > inds.EMA50 {
		// Price pulled back to EMA21
		currentPrice := ticks[len(ticks)-1].Price
		distanceToEMA := math.Abs((currentPrice - inds.EMA21) / inds.EMA21)

		if distanceToEMA < 0.001 && inds.RSI > 40 && inds.RSI < 60 {
			confidence := 0.68 * sessionMult

			// Momentum turning back positive
			if inds.Momentum > 0 && inds.Momentum < 0.002 {
				confidence += 0.05
			}

			signal.Direction = "UP"
			signal.Confidence = math.Min(0.78, confidence)
			signal.Reason = "Pullback to EMA in uptrend"
			return signal
		}
	}

	// Downtrend with pullback
	if inds.EMA9 < inds.EMA21 && inds.EMA21 < inds.EMA50 {
		// Price pulled back to EMA21
		currentPrice := ticks[len(ticks)-1].Price
		distanceToEMA := math.Abs((currentPrice - inds.EMA21) / inds.EMA21)

		if distanceToEMA < 0.001 && inds.RSI > 40 && inds.RSI < 60 {
			confidence := 0.68 * sessionMult

			// Momentum turning back negative
			if inds.Momentum < 0 && inds.Momentum > -0.002 {
				confidence += 0.05
			}

			signal.Direction = "DOWN"
			signal.Confidence = math.Min(0.78, confidence)
			signal.Reason = "Pullback to EMA in downtrend"
			return signal
		}
	}

	signal.Direction = "NONE"
	return signal
}

// rangeSignal - Trade ranges when market is consolidating
func (s *ForexStrategy) rangeSignal(currentPrice float64, inds types.Indicators) types.StrategySignal {
	signal := types.StrategySignal{
		Name:   "Range",
		Weight: 0.25,
	}

	// EMAs flat (no trend)
	emaSpread := math.Abs(inds.EMA9 - inds.EMA21)
	if emaSpread < inds.EMA21*0.0008 {
		// Near lower BB - buy
		if inds.BBPosition < -0.70 && inds.RSI < 40 {
			signal.Direction = "UP"
			signal.Confidence = 0.67
			signal.Reason = "Range trading - near support"
			return signal
		}

		// Near upper BB - sell
		if inds.BBPosition > 0.70 && inds.RSI > 60 {
			signal.Direction = "DOWN"
			signal.Confidence = 0.67
			signal.Reason = "Range trading - near resistance"
			return signal
		}
	}

	signal.Direction = "NONE"
	return signal
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

	// Find swing highs and lows
	for i := 5; i < len(recentTicks)-5; i++ {
		price := recentTicks[i].Price

		// Check if it's a swing high
		isHigh := true
		for j := i - 5; j <= i+5; j++ {
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
		for j := i - 5; j <= i+5; j++ {
			if j != i && recentTicks[j].Price < price {
				isLow = false
				break
			}
		}
		if isLow {
			levels.Support = append(levels.Support, price)
		}
	}

	// Keep only recent levels (last 3 of each)
	if len(levels.Support) > 3 {
		levels.Support = levels.Support[len(levels.Support)-3:]
	}
	if len(levels.Resistance) > 3 {
		levels.Resistance = levels.Resistance[len(levels.Resistance)-3:]
	}

	return levels
}

// IsForexMarket checks if market is a forex pair
func IsForexMarket(market string) bool {
	market = strings.ToUpper(market)
	forexPairs := []string{"AUD", "EUR", "GBP", "USD", "JPY", "CHF", "CAD", "NZD"}

	for _, currency := range forexPairs {
		if strings.Contains(market, currency) {
			// Check if it's actually a forex pair (contains two currencies)
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
