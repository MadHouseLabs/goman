package registry

import (
	"fmt"
	"os"

	"github.com/madhouselabs/goman/pkg/provider"
	"github.com/madhouselabs/goman/pkg/provider/aws"
)

// GetProvider returns a provider instance based on type
func GetProvider(providerType, profile, region string) (provider.Provider, error) {
	switch providerType {
	case "aws":
		return aws.NewProvider(profile, region)
	case "gcp":
		// return gcp.NewProvider(profile, region)
		return nil, fmt.Errorf("GCP provider not yet implemented")
	case "azure":
		// return azure.NewProvider(profile, region)
		return nil, fmt.Errorf("Azure provider not yet implemented")
	default:
		return nil, fmt.Errorf("unknown provider: %s", providerType)
	}
}

// GetDefaultProvider returns the default provider based on environment
func GetDefaultProvider() (provider.Provider, error) {
	// Detect provider from environment
	providerType := DetectProviderFromEnvironment()

	// Get profile
	profile := os.Getenv("CLOUD_PROFILE")
	if profile == "" && providerType == "aws" {
		profile = os.Getenv("AWS_PROFILE")
	}

	// Get region
	region := os.Getenv("CLOUD_REGION")
	if region == "" && providerType == "aws" {
		region = os.Getenv("AWS_REGION")
		if region == "" {
			region = "ap-south-1"
		}
	}

	return GetProvider(providerType, profile, region)
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
