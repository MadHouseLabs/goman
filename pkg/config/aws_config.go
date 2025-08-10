package config

import "os"

const (
	// DefaultAWSRegion is the standard region for all AWS operations (Mumbai, India)
	DefaultAWSRegion = "ap-south-1"
)

// GetDefaultRegion returns the default region for the current provider
func GetDefaultRegion() string {
	// Check provider-agnostic environment variable first
	if region := os.Getenv("CLOUD_REGION"); region != "" {
		return region
	}

	// Fall back to provider-specific variables
	if region := os.Getenv("AWS_REGION"); region != "" {
		return region
	}

	// Default to Mumbai region for AWS
	return DefaultAWSRegion
}

// GetAWSRegion returns the AWS region to use (deprecated, use GetDefaultRegion)
func GetAWSRegion() string {
	return GetDefaultRegion()
}

// GetProviderCredentials returns the credentials/profile for the specified provider
func GetProviderCredentials(providerType string) string {
	// In serverless environments, we don't use profiles
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		return ""
	}

	switch providerType {
	case "aws":
		if profile := os.Getenv("AWS_PROFILE"); profile != "" {
			return profile
		}
		return "default"
	case "gcp":
		return os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	default:
		return ""
	}
}

// GetAWSProfile returns the AWS profile to use (deprecated, use GetProviderCredentials)
func GetAWSProfile() string {
	return GetProviderCredentials("aws")
}
