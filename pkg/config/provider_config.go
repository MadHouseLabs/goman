package config

import (
	"fmt"
	"os"
	"sync"
)

// ProviderConfig holds provider-specific configuration
type ProviderConfig struct {
	mu sync.RWMutex

	// AWS specific
	AWSAMIID         string
	AWSInstanceType  string
	AWSKeyNamePrefix string

	// Common
	DefaultNodeCount  int
	DefaultK3sVersion string
	NetworkVPCCIDR    string
	NetworkSubnetCIDR string
}

var (
	providerConfig *ProviderConfig
	configOnce     sync.Once
)

// GetProviderConfig returns the singleton provider configuration
func GetProviderConfig() *ProviderConfig {
	configOnce.Do(func() {
		providerConfig = &ProviderConfig{
			// AWS defaults
			AWSAMIID:         getProviderEnvOrDefault("GOMAN_AWS_AMI_ID", ""),
			AWSInstanceType:  getProviderEnvOrDefault("GOMAN_AWS_INSTANCE_TYPE", "t3.medium"),
			AWSKeyNamePrefix: getProviderEnvOrDefault("GOMAN_AWS_KEY_PREFIX", "goman"),

			// Common defaults
			DefaultNodeCount:  getEnvIntOrDefault("GOMAN_DEFAULT_NODE_COUNT", 3),
			DefaultK3sVersion: getProviderEnvOrDefault("GOMAN_K3S_VERSION", "v1.28.5+k3s1"),
			NetworkVPCCIDR:    getProviderEnvOrDefault("GOMAN_VPC_CIDR", "10.0.0.0/16"),
			NetworkSubnetCIDR: getProviderEnvOrDefault("GOMAN_SUBNET_CIDR", "10.0.1.0/24"),
		}

		// Auto-detect AMI if not set
		if providerConfig.AWSAMIID == "" {
			providerConfig.AWSAMIID = getDefaultAMIForRegion(GetAWSRegion())
		}
	})
	return providerConfig
}

// getDefaultAMIForRegion returns the default Ubuntu 22.04 AMI for a region
func getDefaultAMIForRegion(region string) string {
	// These are Ubuntu 22.04 LTS AMIs
	// In production, this should be fetched dynamically
	amiMap := map[string]string{
		"ap-south-1":     "ami-0f5ee92e2d63afc18", // Mumbai
		"us-east-1":      "ami-0c55b159cbfafe1f0", // N. Virginia
		"us-west-2":      "ami-0a634ae95e11c6ba9", // Oregon
		"eu-west-1":      "ami-0694d931cee176e7d", // Ireland
		"ap-southeast-1": "ami-0df7a207adb9748c7", // Singapore
	}

	if ami, ok := amiMap[region]; ok {
		return ami
	}

	// Fallback to a generic AMI ID that user must override
	return "ami-ubuntu-22.04"
}

// getProviderEnvOrDefault returns environment variable or default value
func getProviderEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvIntOrDefault returns environment variable as int or default value
func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		// Simple conversion, in production should handle errors
		var result int
		if _, err := fmt.Sscanf(value, "%d", &result); err == nil {
			return result
		}
	}
	return defaultValue
}

// GetProviderImageID returns the configured image ID for the specified provider
func (c *ProviderConfig) GetProviderImageID(providerType string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	switch providerType {
	case "aws":
		return c.AWSAMIID
	default:
		return ""
	}
}

// GetAWSAMI returns the configured AMI ID for AWS (deprecated, use GetProviderImageID)
func (c *ProviderConfig) GetAWSAMI() string {
	return c.GetProviderImageID("aws")
}

// SetProviderImageID sets the image ID for the specified provider
func (c *ProviderConfig) SetProviderImageID(providerType, imageID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch providerType {
	case "aws":
		c.AWSAMIID = imageID
	}
}

// Validate checks if the configuration is valid
func (c *ProviderConfig) Validate() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.AWSAMIID == "" || c.AWSAMIID == "ami-ubuntu-22.04" {
		return fmt.Errorf("AWS AMI ID not configured. Please set GOMAN_AWS_AMI_ID environment variable")
	}

	if c.DefaultNodeCount < 1 {
		return fmt.Errorf("invalid node count: %d", c.DefaultNodeCount)
	}

	return nil
}
