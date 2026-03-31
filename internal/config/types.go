package config

// Config represents the root of stasis.yaml
type Config struct {
	Name     string             `mapstructure:"name"`
	Version  string             `mapstructure:"version"`
	Remote   RemoteConfig       `mapstructure:"remote"`
	Services map[string]Service `mapstructure:"services"` // Map keys are service names (e.g., "postgres")
}

type RemoteConfig struct {
	S3 S3Config `mapstructure:"s3"`
}

// S3Config holds AWS S3 specific settings
type S3Config struct {
	Bucket string `mapstructure:"bucket"`
	Region string `mapstructure:"region"`
}

// Service represents a single container definition
type Service struct {
	Image       string   `mapstructure:"image"`        // e.g., "postgres:14"
	Ports       []string `mapstructure:"ports"`        // e.g., ["5432:5432"]
	Environment []string `mapstructure:"environment"`  // e.g., ["POSTGRES_PASSWORD=secret"]
	Volumes     []string `mapstructure:"volumes"`      // e.g., ["pgdata:/var/lib/postgresql/data"]
}