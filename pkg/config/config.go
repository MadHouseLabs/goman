package config

import (
	"os"
	"path/filepath"
)

// Config represents application configuration
type Config struct {
	AWSProfile      string
	AWSRegion       string
	DefaultProvider string
	InstanceType    string
	K3sVersion      string
	SSHKeyPath      string
}

// NewConfig creates a new configuration with defaults
func NewConfig() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	return &Config{
		AWSProfile:      getEnvOrDefault("AWS_PROFILE", "default"),
		AWSRegion:       getEnvOrDefault("AWS_REGION", "ap-south-1"),
		DefaultProvider: "AWS",
		InstanceType:    "t3.medium",
		K3sVersion:      "v1.28.5+k3s1",
		SSHKeyPath:      filepath.Join(homeDir, ".ssh", "id_rsa"),
	}, nil
}

// LoadFromFile loads configuration from a file (not implemented - uses env vars)
func LoadFromFile(path string) (*Config, error) {
	// Configuration is managed through environment variables
	return NewConfig()
}

// Save saves configuration to a file (not implemented - uses env vars)
func (c *Config) Save(path string) error {
	// Configuration is managed through environment variables
	return nil
}

// getEnvOrDefault returns environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
