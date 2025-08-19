package setup

import (
	"context"
	"fmt"

	"github.com/madhouselabs/goman/pkg/provider/registry"
	"github.com/madhouselabs/goman/pkg/storage"
)

// InitializeResult contains the results of initialization
type InitializeResult struct {
	ProviderType      string
	StorageReady      bool
	FunctionReady     bool
	LockServiceReady  bool
	NotificationsReady bool
	AuthReady         bool
	Resources         map[string]string
	Errors            []string
	
}

// EnsureFullSetup ensures all required resources are properly configured for the provider
func EnsureFullSetup(ctx context.Context) (*InitializeResult, error) {
	provider, err := registry.GetDefaultProvider()
	if err != nil {
		return &InitializeResult{
			Errors: []string{fmt.Sprintf("Provider setup failed: %v", err)},
		}, fmt.Errorf("failed to get provider: %w", err)
	}
	
	// Initialize storage with the provider
	_, err = storage.NewStorageWithProvider(provider)
	if err != nil {
		return &InitializeResult{
			Errors: []string{fmt.Sprintf("Storage setup failed: %v", err)},
		}, fmt.Errorf("failed to initialize storage: %w", err)
	}

	providerResult, err := provider.Initialize(ctx)
	if err != nil {
		return &InitializeResult{
			ProviderType: provider.Name(),
			Errors:       []string{fmt.Sprintf("Provider initialization failed: %v", err)},
		}, fmt.Errorf("provider initialization failed: %w", err)
	}

	result := &InitializeResult{
		ProviderType:       providerResult.ProviderType,
		StorageReady:       providerResult.StorageReady,
		FunctionReady:      providerResult.FunctionReady,
		LockServiceReady:   providerResult.LockServiceReady,
		NotificationsReady: providerResult.NotificationsReady,
		AuthReady:          providerResult.AuthReady,
		Resources:          providerResult.Resources,
		Errors:             providerResult.Errors,
		
	}

	return result, nil
}

// CleanupInfrastructure removes all provider-specific infrastructure
func CleanupInfrastructure(ctx context.Context) error {
	// Get the provider
	provider, err := registry.GetDefaultProvider()
	if err != nil {
		return fmt.Errorf("failed to get provider: %w", err)
	}

	// Let the provider handle its own cleanup
	if err := provider.Cleanup(ctx); err != nil {
		return fmt.Errorf("provider cleanup failed: %w", err)
	}

	return nil
}
