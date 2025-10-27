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

	// Validate config
	if err := validate(config); err != nil {
		return config, fmt.Errorf("invalid config: %w", err)
	}

	return config, nil
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

	return nil
}
