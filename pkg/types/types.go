package types

import "time"

// Tick represents a single price point
type Tick struct {
	Market    string    `json:"market"`
	Price     float64   `json:"price"`
	Timestamp time.Time `json:"timestamp"`
	Epoch     int64     `json:"epoch"`
}

// Candle represents OHLCV data (optional, for aggregation)
type Candle struct {
	Market    string    `json:"market"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
	Timestamp time.Time `json:"timestamp"`
}

// Indicators holds calculated technical indicators
type Indicators struct {
	RSI           float64 `json:"rsi"`
	EMA9          float64 `json:"ema_9"`
	EMA21         float64 `json:"ema_21"`
	EMA50         float64 `json:"ema_50"`
	BBUpper       float64 `json:"bb_upper"`
	BBMiddle      float64 `json:"bb_middle"`
	BBLower       float64 `json:"bb_lower"`
	BBPosition    float64 `json:"bb_position"` // -1 to 1
	Volatility    float64 `json:"volatility"`
	Momentum      float64 `json:"momentum"`
	TrendStrength float64 `json:"trend_strength"`
}

// Prediction represents a trading prediction
type Prediction struct {
	ID           string     `json:"id"`
	Market       string     `json:"market"`
	MarketType   string     `json:"market_type"` // "forex", "volatility", "crash_boom"
	Direction    string     `json:"direction"`   // "UP", "DOWN", "NONE"
	Confidence   float64    `json:"confidence"`
	Reason       string     `json:"reason"`
	CurrentPrice float64    `json:"current_price"`
	Duration     int        `json:"duration"` // seconds
	Timestamp    time.Time  `json:"timestamp"`
	Indicators   Indicators `json:"indicators"`
	DataPoints   int        `json:"data_points"`
}

// PendingPrediction tracks a prediction waiting for outcome
type PendingPrediction struct {
	ID         string
	Market     string
	MarketType string
	Direction  string
	EntryPrice float64
	EntryTime  time.Time
	Duration   int
	Confidence float64
	ExpiryTime time.Time
}

// TradeResult stores the outcome of a prediction
type TradeResult struct {
	PredictionID string    `json:"prediction_id"`
	Market       string    `json:"market"`
	MarketType   string    `json:"market_type"`
	Direction    string    `json:"direction"`
	EntryPrice   float64   `json:"entry_price"`
	ExitPrice    float64   `json:"exit_price"`
	EntryTime    time.Time `json:"entry_time"`
	ExitTime     time.Time `json:"exit_time"`
	Duration     int       `json:"duration"`
	Confidence   float64   `json:"confidence"`
	Won          bool      `json:"won"`
	ProfitLoss   float64   `json:"profit_loss"`
	PriceChange  float64   `json:"price_change"`
}

// Stats represents performance statistics
type Stats struct {
	Market          string        `json:"market"`
	MarketType      string        `json:"market_type"`
	TotalTrades     int           `json:"total_trades"`
	Wins            int           `json:"wins"`
	Losses          int           `json:"losses"`
	WinRate         float64       `json:"win_rate"`
	TotalProfitLoss float64       `json:"total_profit_loss"`
	AvgConfidence   float64       `json:"avg_confidence"`
	BestStreak      int           `json:"best_streak"`
	CurrentStreak   int           `json:"current_streak"`
	LastUpdated     time.Time     `json:"last_updated"`
	RecentTrades    []TradeResult `json:"recent_trades,omitempty"`
}

// MarketData holds all data for a single market
type MarketData struct {
	Market     string
	MarketType string
	Ticks      []Tick
	LastUpdate time.Time
	IsActive   bool
}

// StrategySignal represents a signal from a single strategy
type StrategySignal struct {
	Name       string
	Direction  string // "UP", "DOWN", "NONE"
	Confidence float64
	Reason     string
	Weight     float64
}

// Config represents application configuration
type Config struct {
	Mode             string           `yaml:"mode"`    // "synthetics", "forex", "both"
	Markets          []string         `yaml:"markets"` // Deprecated - use specific lists
	SyntheticMarkets []string         `yaml:"synthetic_markets"`
	ForexMarkets     []string         `yaml:"forex_markets"`
	DataSource       DataSourceConfig `yaml:"datasource"`
	Strategy         StrategyConfig   `yaml:"strategy"`
	Risk             RiskConfig       `yaml:"risk"`
	Storage          StorageConfig    `yaml:"storage"`
	API              APIConfig        `yaml:"api"`
	Logging          LoggingConfig    `yaml:"logging"`
	Tracking         TrackingConfig   `yaml:"tracking"`
}

type DataSourceConfig struct {
	APIURL         string `yaml:"api_url"`
	ReconnectDelay int    `yaml:"reconnect_delay"`
	PingInterval   int    `yaml:"ping_interval"`
}

type StrategyConfig struct {
	MinConfidence float64           `yaml:"min_confidence"`
	RSIPeriod     int               `yaml:"rsi_period"`
	RSIOverbought float64           `yaml:"rsi_overbought"`
	RSIOversold   float64           `yaml:"rsi_oversold"`
	EMAFast       int               `yaml:"ema_fast"`
	EMASlow       int               `yaml:"ema_slow"`
	EMATrend      int               `yaml:"ema_trend"`
	BBPeriod      int               `yaml:"bb_period"`
	BBStdDev      float64           `yaml:"bb_std_dev"`
	Volatility    VolatilityWeights `yaml:"volatility"`
	CrashBoom     CrashBoomWeights  `yaml:"crash_boom"`
	Forex         ForexWeights      `yaml:"forex"`
}

type VolatilityWeights struct {
	MeanReversionWeight float64 `yaml:"mean_reversion_weight"`
	MomentumWeight      float64 `yaml:"momentum_weight"`
	PatternWeight       float64 `yaml:"pattern_weight"`
}

type CrashBoomWeights struct {
	SpikeDetectionWeight float64 `yaml:"spike_detection_weight"`
	TrendWeight          float64 `yaml:"trend_weight"`
	VolatilityWeight     float64 `yaml:"volatility_weight"`
}

type ForexWeights struct {
	TrendFollowingWeight    float64 `yaml:"trend_following_weight"`
	SupportResistanceWeight float64 `yaml:"support_resistance_weight"`
	EMACrossoverWeight      float64 `yaml:"ema_crossover_weight"`
	PullbackWeight          float64 `yaml:"pullback_weight"`
	RangeWeight             float64 `yaml:"range_weight"`
}

type RiskConfig struct {
	MaxPredictionsPerMinute     int            `yaml:"max_predictions_per_minute"`
	MinTicksRequired            int            `yaml:"min_ticks_required"`
	SkipHighVolatilityThreshold float64        `yaml:"skip_high_volatility_threshold"`
	MaxSpreadPips               float64        `yaml:"max_spread_pips"`
	Synthetics                  SyntheticsRisk `yaml:"synthetics"`
	Forex                       ForexRisk      `yaml:"forex"`
}

type SyntheticsRisk struct {
	MaxPredictionsPerMinute     int     `yaml:"max_predictions_per_minute"`
	MinTicksRequired            int     `yaml:"min_ticks_required"`
	SkipHighVolatilityThreshold float64 `yaml:"skip_high_volatility_threshold"`
	PreferredDuration           int     `yaml:"preferred_duration"`
}

type ForexRisk struct {
	MaxPredictionsPerMinute     int     `yaml:"max_predictions_per_minute"`
	MinTicksRequired            int     `yaml:"min_ticks_required"`
	SkipHighVolatilityThreshold float64 `yaml:"skip_high_volatility_threshold"`
	PreferredDuration           int     `yaml:"preferred_duration"`
	AvoidAsianSession           bool    `yaml:"avoid_asian_session"`
	BoostLondonNYOverlap        bool    `yaml:"boost_london_ny_overlap"`
}

type StorageConfig struct {
	MaxTicksInMemory     int `yaml:"max_ticks_in_memory"`
	KeepPredictionsHours int `yaml:"keep_predictions_hours"`
	AutoCleanupInterval  int `yaml:"auto_cleanup_interval"`
}

type APIConfig struct {
	Host             string `yaml:"host"`
	Port             int    `yaml:"port"`
	EnableCORS       bool   `yaml:"enable_cors"`
	WebSocketEnabled bool   `yaml:"websocket_enabled"`
	MaxConnections   int    `yaml:"max_connections"`
}

type LoggingConfig struct {
	Level   string `yaml:"level"`
	File    string `yaml:"file"`
	Console bool   `yaml:"console"`
}

type TrackingConfig struct {
	CalculateStatsInterval int  `yaml:"calculate_stats_interval"`
	MinTradesForStats      int  `yaml:"min_trades_for_stats"`
	DisplayRecentTrades    int  `yaml:"display_recent_trades"`
	SeparateStatsByType    bool `yaml:"separate_stats_by_type"`
}
