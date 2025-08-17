package provider

import (
	"context"
)

// DNSService provides DNS management for cluster API endpoints
// This abstraction allows for different DNS providers (Route53, CloudDNS, etc.)
type DNSService interface {
	// Initialize ensures the DNS backend exists (e.g., hosted zone)
	// config contains provider-specific configuration like network IDs
	Initialize(ctx context.Context, config map[string]string) error

	// CreateRecordSet creates a new DNS record set
	// domain: the domain name (e.g., "k3s-cluster1.internal")
	// recordType: the DNS record type (e.g., "A", "CNAME")
	// records: the IP addresses or values for the record
	// ttl: time to live in seconds
	CreateRecordSet(ctx context.Context, domain string, recordType string, records []string, ttl int) error

	// UpdateRecordSet updates an existing DNS record set
	UpdateRecordSet(ctx context.Context, domain string, recordType string, records []string, ttl int) error

	// DeleteRecordSet removes a DNS record set
	DeleteRecordSet(ctx context.Context, domain string, recordType string) error

	// GetRecordSet retrieves the current values for a DNS record
	GetRecordSet(ctx context.Context, domain string, recordType string) ([]string, error)

	// GetZoneName returns the DNS zone name being managed
	GetZoneName() string
}

// DNSConfig represents configuration for DNS service
type DNSConfig struct {
	ZoneName   string // The DNS zone to manage (e.g., "goman.internal")
	PrivateZone bool   // Whether this is a private/internal zone
	VPCID      string // VPC to associate with private zone (AWS-specific)
	Region     string // Region for the DNS service
}