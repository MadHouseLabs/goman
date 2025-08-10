package config

import "os"

const (
	// DefaultAWSRegion is the standard region for all AWS operations (Mumbai, India)
	DefaultAWSRegion = "ap-south-1"
)

// GetAWSRegion returns the AWS region to use, defaulting to ap-south-1 (Mumbai)
func GetAWSRegion() string {
	// Allow override via environment variable if needed
	region := os.Getenv("AWS_REGION")
	if region != "" {
		return region
	}
	
	// Always default to Mumbai region
	return DefaultAWSRegion
}

// GetAWSProfile returns the AWS profile to use
func GetAWSProfile() string {
	// In Lambda environment, we don't use profiles
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		return ""
	}
	
	profile := os.Getenv("AWS_PROFILE")
	if profile == "" {
		profile = "default"
	}
	return profile
}