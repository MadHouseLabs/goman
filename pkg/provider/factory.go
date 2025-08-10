package provider

import (
	"os"
)

// Factory creates providers based on configuration
type Factory struct {
	defaultProvider string
	defaultRegion   string
}

// NewFactory creates a new provider factory
func NewFactory() *Factory {
	// Detect provider from environment
	provider := os.Getenv("CLOUD_PROVIDER")
	if provider == "" {
		// Try to detect from AWS environment
		if os.Getenv("AWS_PROFILE") != "" || os.Getenv("AWS_REGION") != "" {
			provider = "aws"
		} else {
			provider = "aws" // Default to AWS for now
		}
	}
	
	region := os.Getenv("CLOUD_REGION")
	if region == "" {
		region = os.Getenv("AWS_REGION")
		if region == "" {
			region = "ap-south-1" // Default region
		}
	}
	
	return &Factory{
		defaultProvider: provider,
		defaultRegion:   region,
	}
}

// GetProviderType returns the default provider type
func (f *Factory) GetProviderType() string {
	return f.defaultProvider
}

// GetRegion returns the default region
func (f *Factory) GetRegion() string {
	return f.defaultRegion
}

// DetectProviderFromEnvironment detects the cloud provider from environment
func DetectProviderFromEnvironment() string {
	// Check explicit setting
	if provider := os.Getenv("CLOUD_PROVIDER"); provider != "" {
		return provider
	}
	
	// Check for AWS
	if os.Getenv("AWS_PROFILE") != "" || os.Getenv("AWS_REGION") != "" || os.Getenv("AWS_DEFAULT_REGION") != "" {
		return "aws"
	}
	
	// Check for GCP
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" || os.Getenv("GCP_PROJECT") != "" {
		return "gcp"
	}
	
	// Check for Azure
	if os.Getenv("AZURE_SUBSCRIPTION_ID") != "" || os.Getenv("AZURE_TENANT_ID") != "" {
		return "azure"
	}
	
	// Default to AWS
	return "aws"
}

// GetFunctionPackagePath returns the path to the function package for a provider
func GetFunctionPackagePath(providerType string) string {
	if providerType == "" {
		providerType = DetectProviderFromEnvironment()
	}
	
	switch providerType {
	case "aws":
		return "build/lambda-aws-controller.zip"
	case "gcp":
		return "build/function-gcp-controller.zip"
	case "azure":
		return "build/function-azure-controller.zip"
	default:
		return "build/function-controller.zip"
	}
}