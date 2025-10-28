package config

import (
	"fmt"
	"os"

	"otc-predictor/pkg/types"

	"gopkg.in/yaml.v3"
)

// Load loads configuration from file
func Load(filename string) (types.Config, error) {
	var config types.Config

	data, err := os.ReadFile(filename)
	if err != nil {
		return config, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("failed to parse config: %w", err)
	}

	// Process multi-mode markets
	if err := processMarkets(&config); err != nil {
		return config, fmt.Errorf("failed to process markets: %w", err)
	}

	// Set defaults for missing values
	setDefaults(&config)

	// Validate config
	if err := validate(config); err != nil {
		return config, fmt.Errorf("invalid config: %w", err)
	}

	return config, nil
}

// processMarkets handles the new multi-mode market configuration
func processMarkets(config *types.Config) error {
	// If using new format (synthetic_markets + forex_markets)
	if len(config.SyntheticMarkets) > 0 || len(config.ForexMarkets) > 0 {
		// Set default mode if not specified
		if config.Mode == "" {
			config.Mode = "both"
		}

		// Merge markets based on mode
		switch config.Mode {
		case "synthetics":
			config.Markets = make([]string, len(config.SyntheticMarkets))
			copy(config.Markets, config.SyntheticMarkets)

		case "forex":
			config.Markets = make([]string, len(config.ForexMarkets))
			copy(config.Markets, config.ForexMarkets)

		case "both":
			// Combine both lists
			config.Markets = make([]string, 0, len(config.SyntheticMarkets)+len(config.ForexMarkets))
			config.Markets = append(config.Markets, config.SyntheticMarkets...)
			config.Markets = append(config.Markets, config.ForexMarkets...)

		default:
			return fmt.Errorf("invalid mode '%s' (must be 'synthetics', 'forex', or 'both')", config.Mode)
		}
	} else if len(config.Markets) == 0 {
		return fmt.Errorf("no markets configured (need either 'markets', 'synthetic_markets', or 'forex_markets')")
	}

	return nil
}

// setDefaults sets default values for missing config fields
func setDefaults(config *types.Config) {
	// Mode default
	if config.Mode == "" {
		config.Mode = "both"
	}

	// API defaults
	if config.API.Host == "" {
		config.API.Host = "0.0.0.0"
	}
	if config.API.Port == 0 {
		config.API.Port = 8080
	}
	if !config.API.EnableCORS {
		config.API.EnableCORS = true
	}
	if !config.API.WebSocketEnabled {
		config.API.WebSocketEnabled = true
	}
	if config.API.MaxConnections == 0 {
		config.API.MaxConnections = 50
	}

	// DataSource defaults
	if config.DataSource.ReconnectDelay == 0 {
		config.DataSource.ReconnectDelay = 5
	}
	if config.DataSource.PingInterval == 0 {
		config.DataSource.PingInterval = 25
	}

	// Strategy defaults
	if config.Strategy.MinConfidence == 0 {
		config.Strategy.MinConfidence = 0.70
	}
	if config.Strategy.RSIPeriod == 0 {
		config.Strategy.RSIPeriod = 14
	}
	if config.Strategy.RSIOverbought == 0 {
		config.Strategy.RSIOverbought = 72
	}
	if config.Strategy.RSIOversold == 0 {
		config.Strategy.RSIOversold = 28
	}
	if config.Strategy.EMAFast == 0 {
		config.Strategy.EMAFast = 9
	}
	if config.Strategy.EMASlow == 0 {
		config.Strategy.EMASlow = 21
	}
	if config.Strategy.EMATrend == 0 {
		config.Strategy.EMATrend = 50
	}
	if config.Strategy.BBPeriod == 0 {
		config.Strategy.BBPeriod = 20
	}
	if config.Strategy.BBStdDev == 0 {
		config.Strategy.BBStdDev = 2.0
	}

	// Risk defaults - global
	if config.Risk.MaxPredictionsPerMinute == 0 {
		config.Risk.MaxPredictionsPerMinute = 30
	}
	if config.Risk.MinTicksRequired == 0 {
		config.Risk.MinTicksRequired = 60
	}
	if config.Risk.SkipHighVolatilityThreshold == 0 {
		config.Risk.SkipHighVolatilityThreshold = 0.03
	}
	if config.Risk.MaxSpreadPips == 0 {
		config.Risk.MaxSpreadPips = 2.5
	}

	// Synthetics risk defaults
	if config.Risk.Synthetics.MaxPredictionsPerMinute == 0 {
		config.Risk.Synthetics.MaxPredictionsPerMinute = 30
	}
	if config.Risk.Synthetics.MinTicksRequired == 0 {
		config.Risk.Synthetics.MinTicksRequired = 60
	}
	if config.Risk.Synthetics.SkipHighVolatilityThreshold == 0 {
		config.Risk.Synthetics.SkipHighVolatilityThreshold = 0.03
	}
	if config.Risk.Synthetics.PreferredDuration == 0 {
		config.Risk.Synthetics.PreferredDuration = 60
	}

	// Forex risk defaults
	if config.Risk.Forex.MaxPredictionsPerMinute == 0 {
		config.Risk.Forex.MaxPredictionsPerMinute = 20
	}
	if config.Risk.Forex.MinTicksRequired == 0 {
		config.Risk.Forex.MinTicksRequired = 80
	}
	if config.Risk.Forex.SkipHighVolatilityThreshold == 0 {
		config.Risk.Forex.SkipHighVolatilityThreshold = 0.025
	}
	if config.Risk.Forex.PreferredDuration == 0 {
		config.Risk.Forex.PreferredDuration = 180
	}

	// Storage defaults
	if config.Storage.MaxTicksInMemory == 0 {
		config.Storage.MaxTicksInMemory = 500
	}
	if config.Storage.KeepPredictionsHours == 0 {
		config.Storage.KeepPredictionsHours = 12
	}
	if config.Storage.AutoCleanupInterval == 0 {
		config.Storage.AutoCleanupInterval = 1800
	}

	// Tracking defaults
	if config.Tracking.CalculateStatsInterval == 0 {
		config.Tracking.CalculateStatsInterval = 60
	}
	if config.Tracking.MinTradesForStats == 0 {
		config.Tracking.MinTradesForStats = 5
	}
	if config.Tracking.DisplayRecentTrades == 0 {
		config.Tracking.DisplayRecentTrades = 20
	}

	// Logging defaults
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}
	if config.Logging.File == "" {
		config.Logging.File = "otc-predictor.log"
	}
	// Console logging defaults to true
	if !config.Logging.Console {
		config.Logging.Console = true
	}
}

// validate validates configuration
func validate(config types.Config) error {
	if len(config.Markets) == 0 {
		return fmt.Errorf("no markets configured")
	}

	if config.DataSource.APIURL == "" {
		return fmt.Errorf("data source API URL is required")
	}

	if config.Strategy.MinConfidence < 0 || config.Strategy.MinConfidence > 1 {
		return fmt.Errorf("min_confidence must be between 0 and 1")
	}

	if config.API.Port < 1 || config.API.Port > 65535 {
		return fmt.Errorf("invalid API port")
	}

	// Validate mode
	validModes := map[string]bool{"synthetics": true, "forex": true, "both": true}
	if !validModes[config.Mode] {
		return fmt.Errorf("invalid mode '%s' (must be 'synthetics', 'forex', or 'both')", config.Mode)
	}

	return nil
}
