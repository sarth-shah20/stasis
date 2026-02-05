package config

import (
	"fmt"
	"github.com/spf13/viper"
)

// Load reads the configuration from the given filename (e.g., "stasis.yaml")
// It returns a pointer to the Config struct or an error if something fails.
func Load(filename string) (*Config, error) {
	// 1. Tell Viper what file to look for
	viper.SetConfigFile(filename)

	// 2. Try to read the file from disk
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return nil, fmt.Errorf("config file %s not found. Run 'stasis init' to create one", filename)
		}
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// 3. Create an empty Config struct
	var cfg Config

	// 4. Unmarshal: Viper fills the struct fields based on the mapstructure tags
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode into struct: %w", err)
	}

	return &cfg, nil
}