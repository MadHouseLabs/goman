package provider

// ServiceType represents generic cloud service categories
// This abstraction allows the controller layer to work with any cloud provider
// without being tightly coupled to specific provider services (like AWS EC2, S3, etc.)
type ServiceType string

const (
	// ServiceTypeCompute represents compute/VM services (AWS EC2, Azure VM, GCP Compute Engine)
	ServiceTypeCompute ServiceType = "COMPUTE"
	
	// ServiceTypeStorage represents object storage services (AWS S3, Azure Blob, GCP Cloud Storage)
	ServiceTypeStorage ServiceType = "STORAGE"
	
	// ServiceTypeCommand represents remote command execution services (AWS SSM, Azure Run Command, GCP OS Login)
	ServiceTypeCommand ServiceType = "COMMAND"
	
	// ServiceTypeLock represents distributed locking services (AWS DynamoDB, Azure Cosmos DB, GCP Firestore)
	ServiceTypeLock ServiceType = "LOCK"
	
	// ServiceTypeFunction represents serverless function services (AWS Lambda, Azure Functions, GCP Cloud Functions)
	ServiceTypeFunction ServiceType = "FUNCTION"
	
	// ServiceTypeNotification represents messaging/notification services (AWS SNS/SQS, Azure Service Bus, GCP Pub/Sub)
	ServiceTypeNotification ServiceType = "NOTIFICATION"
)

// AllServiceTypes returns all available service types
func AllServiceTypes() []ServiceType {
	return []ServiceType{
		ServiceTypeCompute,
		ServiceTypeStorage,
		ServiceTypeCommand,
		ServiceTypeLock,
		ServiceTypeFunction,
		ServiceTypeNotification,
	}
}

// String returns the string representation of the service type
func (st ServiceType) String() string {
	return string(st)
}

// IsValid checks if the service type is valid
func (st ServiceType) IsValid() bool {
	for _, validType := range AllServiceTypes() {
		if st == validType {
			return true
		}
	}
	return false
}

// ProviderConfig represents provider-specific configuration
type ProviderConfig struct {
	// Default values that can vary by provider
	DefaultInstanceType    string            `json:"defaultInstanceType"`
	DefaultRegion          string            `json:"defaultRegion"`
	DefaultTopics          map[string]string `json:"defaultTopics"`
	ServiceNames           map[ServiceType]string `json:"serviceNames"`
	
	// Provider-specific settings
	CustomSettings         map[string]interface{} `json:"customSettings,omitempty"`
}

// ServiceConfiguration represents configuration for a specific service type
type ServiceConfiguration struct {
	// Generic settings that apply to any provider
	FailureThreshold  int    `json:"failureThreshold"`
	RecoveryTimeoutMS int64  `json:"recoveryTimeoutMs"`
	SuccessThreshold  int    `json:"successThreshold"`
	RequestTimeoutMS  int64  `json:"requestTimeoutMs"`
	MaxConcurrent     int    `json:"maxConcurrent"`
	
	// Provider-specific settings
	ProviderSpecific  map[string]interface{} `json:"providerSpecific,omitempty"`
}

// DefaultServiceConfiguration returns default configuration for a service type
func DefaultServiceConfiguration(serviceType ServiceType) ServiceConfiguration {
	switch serviceType {
	case ServiceTypeCompute:
		return ServiceConfiguration{
			FailureThreshold:  3,
			RecoveryTimeoutMS: 30000, // 30 seconds
			SuccessThreshold:  2,
			RequestTimeoutMS:  45000, // 45 seconds
			MaxConcurrent:     5,
		}
	case ServiceTypeStorage:
		return ServiceConfiguration{
			FailureThreshold:  5,
			RecoveryTimeoutMS: 15000, // 15 seconds
			SuccessThreshold:  3,
			RequestTimeoutMS:  30000, // 30 seconds
			MaxConcurrent:     10,
		}
	case ServiceTypeCommand:
		return ServiceConfiguration{
			FailureThreshold:  4,
			RecoveryTimeoutMS: 45000, // 45 seconds
			SuccessThreshold:  2,
			RequestTimeoutMS:  60000, // 60 seconds
			MaxConcurrent:     3,
		}
	case ServiceTypeLock:
		return ServiceConfiguration{
			FailureThreshold:  5,
			RecoveryTimeoutMS: 20000, // 20 seconds
			SuccessThreshold:  3,
			RequestTimeoutMS:  15000, // 15 seconds
			MaxConcurrent:     8,
		}
	case ServiceTypeFunction:
		return ServiceConfiguration{
			FailureThreshold:  3,
			RecoveryTimeoutMS: 60000, // 60 seconds
			SuccessThreshold:  2,
			RequestTimeoutMS:  30000, // 30 seconds
			MaxConcurrent:     2,
		}
	case ServiceTypeNotification:
		return ServiceConfiguration{
			FailureThreshold:  5,
			RecoveryTimeoutMS: 20000, // 20 seconds
			SuccessThreshold:  3,
			RequestTimeoutMS:  20000, // 20 seconds
			MaxConcurrent:     10,
		}
	default:
		// Default fallback configuration
		return ServiceConfiguration{
			FailureThreshold:  3,
			RecoveryTimeoutMS: 30000,
			SuccessThreshold:  2,
			RequestTimeoutMS:  30000,
			MaxConcurrent:     5,
		}
	}
}