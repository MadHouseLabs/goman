package aws

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// DNSService implements provider.DNSService for AWS Route53
type DNSService struct {
	client      *route53.Client
	hostedZoneID string
	zoneName    string
	accountID   string
	vpcID       string
	region      string
	isPrivate   bool
}

// NewDNSService creates a new Route53 DNS service
func NewDNSService(cfg aws.Config, accountID string, region string) *DNSService {
	return &DNSService{
		client:    route53.NewFromConfig(cfg),
		accountID: accountID,
		region:    region,
		zoneName:  "goman.internal", // Default zone name
		isPrivate: true,              // Default to private zone for internal cluster communication
	}
}

// Initialize ensures the hosted zone exists
// config can contain: "vpc_id" for AWS private zones, "network_id" for other providers
func (d *DNSService) Initialize(ctx context.Context, config map[string]string) error {
	// Extract AWS-specific configuration
	if config != nil {
		if vpcID, ok := config["vpc_id"]; ok && vpcID != "" {
			d.vpcID = vpcID
			log.Printf("[DNS] Using VPC ID from config: %s", vpcID)
		}
	}
	// Check if hosted zone already exists
	existingZone, err := d.findHostedZoneByName(ctx, d.zoneName)
	if err != nil {
		return fmt.Errorf("failed to check for existing hosted zone: %w", err)
	}

	if existingZone != nil {
		d.hostedZoneID = *existingZone.Id
		log.Printf("[DNS] Using existing hosted zone %s for domain %s", d.hostedZoneID, d.zoneName)
		return nil
	}

	// Create new private hosted zone
	log.Printf("[DNS] Creating new private hosted zone for domain %s", d.zoneName)
	
	input := &route53.CreateHostedZoneInput{
		Name:            aws.String(d.zoneName),
		CallerReference: aws.String(fmt.Sprintf("goman-%s-%d", d.accountID, time.Now().Unix())),
	}

	// For private zones, we need VPC association
	if d.isPrivate && d.vpcID != "" {
		input.HostedZoneConfig = &types.HostedZoneConfig{
			PrivateZone: true,
			Comment:     aws.String("Goman K3s cluster internal DNS"),
		}
		input.VPC = &types.VPC{
			VPCId:     aws.String(d.vpcID),
			VPCRegion: types.VPCRegion(d.region),
		}
	}

	result, err := d.client.CreateHostedZone(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create hosted zone: %w", err)
	}

	d.hostedZoneID = *result.HostedZone.Id
	log.Printf("[DNS] Created hosted zone %s for domain %s", d.hostedZoneID, d.zoneName)
	
	// Wait for zone to be created
	time.Sleep(2 * time.Second)
	
	return nil
}

// SetVPC sets the VPC for private zone association
func (d *DNSService) SetVPC(vpcID string) {
	d.vpcID = vpcID
}

// CreateRecordSet creates a new DNS record
func (d *DNSService) CreateRecordSet(ctx context.Context, domain string, recordType string, records []string, ttl int) error {
	if d.hostedZoneID == "" {
		// Try to find the zone if not set
		if err := d.ensureHostedZone(ctx); err != nil {
			return err
		}
	}

	// Ensure domain ends with a dot
	if !strings.HasSuffix(domain, ".") {
		domain = domain + "."
	}

	// Build resource records
	var resourceRecords []types.ResourceRecord
	for _, record := range records {
		resourceRecords = append(resourceRecords, types.ResourceRecord{
			Value: aws.String(record),
		})
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(d.hostedZoneID),
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionCreate,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name:            aws.String(domain),
						Type:            types.RRType(recordType),
						TTL:             aws.Int64(int64(ttl)),
						ResourceRecords: resourceRecords,
					},
				},
			},
			Comment: aws.String(fmt.Sprintf("Goman K3s cluster record for %s", domain)),
		},
	}

	result, err := d.client.ChangeResourceRecordSets(ctx, input)
	if err != nil {
		// If record already exists, try to update it instead
		if strings.Contains(err.Error(), "already exists") {
			log.Printf("[DNS] Record %s already exists, updating instead", domain)
			return d.UpdateRecordSet(ctx, domain, recordType, records, ttl)
		}
		return fmt.Errorf("failed to create DNS record: %w", err)
	}

	log.Printf("[DNS] Created DNS record %s (%s) with values %v, change ID: %s", 
		domain, recordType, records, *result.ChangeInfo.Id)
	
	// Wait for change to propagate
	if err := d.waitForChange(ctx, *result.ChangeInfo.Id); err != nil {
		log.Printf("[DNS] Warning: failed to wait for change propagation: %v", err)
	}
	
	return nil
}

// UpdateRecordSet updates an existing DNS record
func (d *DNSService) UpdateRecordSet(ctx context.Context, domain string, recordType string, records []string, ttl int) error {
	if d.hostedZoneID == "" {
		if err := d.ensureHostedZone(ctx); err != nil {
			return err
		}
	}

	// Ensure domain ends with a dot
	if !strings.HasSuffix(domain, ".") {
		domain = domain + "."
	}

	// Build resource records
	var resourceRecords []types.ResourceRecord
	for _, record := range records {
		resourceRecords = append(resourceRecords, types.ResourceRecord{
			Value: aws.String(record),
		})
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(d.hostedZoneID),
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionUpsert, // Use UPSERT for update
					ResourceRecordSet: &types.ResourceRecordSet{
						Name:            aws.String(domain),
						Type:            types.RRType(recordType),
						TTL:             aws.Int64(int64(ttl)),
						ResourceRecords: resourceRecords,
					},
				},
			},
			Comment: aws.String(fmt.Sprintf("Updated Goman K3s cluster record for %s", domain)),
		},
	}

	result, err := d.client.ChangeResourceRecordSets(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update DNS record: %w", err)
	}

	log.Printf("[DNS] Updated DNS record %s (%s) with values %v", domain, recordType, records)
	
	// Wait for change to propagate
	if err := d.waitForChange(ctx, *result.ChangeInfo.Id); err != nil {
		log.Printf("[DNS] Warning: failed to wait for change propagation: %v", err)
	}
	
	return nil
}

// DeleteRecordSet removes a DNS record
func (d *DNSService) DeleteRecordSet(ctx context.Context, domain string, recordType string) error {
	if d.hostedZoneID == "" {
		if err := d.ensureHostedZone(ctx); err != nil {
			return err
		}
	}

	// First, get the current record to get its values
	records, err := d.GetRecordSet(ctx, domain, recordType)
	if err != nil {
		return fmt.Errorf("failed to get record for deletion: %w", err)
	}

	if len(records) == 0 {
		log.Printf("[DNS] Record %s (%s) not found, nothing to delete", domain, recordType)
		return nil
	}

	// Ensure domain ends with a dot
	if !strings.HasSuffix(domain, ".") {
		domain = domain + "."
	}

	// Build resource records for deletion
	var resourceRecords []types.ResourceRecord
	for _, record := range records {
		resourceRecords = append(resourceRecords, types.ResourceRecord{
			Value: aws.String(record),
		})
	}

	// Get TTL from existing record
	ttl := int64(300) // default TTL
	listInput := &route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(d.hostedZoneID),
		StartRecordName: aws.String(domain),
		StartRecordType: types.RRType(recordType),
		MaxItems:        aws.Int32(1),
	}

	listResult, err := d.client.ListResourceRecordSets(ctx, listInput)
	if err == nil && len(listResult.ResourceRecordSets) > 0 {
		if *listResult.ResourceRecordSets[0].Name == domain {
			ttl = *listResult.ResourceRecordSets[0].TTL
		}
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(d.hostedZoneID),
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionDelete,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name:            aws.String(domain),
						Type:            types.RRType(recordType),
						TTL:             aws.Int64(ttl),
						ResourceRecords: resourceRecords,
					},
				},
			},
			Comment: aws.String(fmt.Sprintf("Delete Goman K3s cluster record for %s", domain)),
		},
	}

	result, err := d.client.ChangeResourceRecordSets(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete DNS record: %w", err)
	}

	log.Printf("[DNS] Deleted DNS record %s (%s)", domain, recordType)
	
	// Wait for change to propagate
	if err := d.waitForChange(ctx, *result.ChangeInfo.Id); err != nil {
		log.Printf("[DNS] Warning: failed to wait for change propagation: %v", err)
	}
	
	return nil
}

// GetRecordSet retrieves the current values for a DNS record
func (d *DNSService) GetRecordSet(ctx context.Context, domain string, recordType string) ([]string, error) {
	if d.hostedZoneID == "" {
		if err := d.ensureHostedZone(ctx); err != nil {
			return nil, err
		}
	}

	// Ensure domain ends with a dot
	if !strings.HasSuffix(domain, ".") {
		domain = domain + "."
	}

	input := &route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(d.hostedZoneID),
		StartRecordName: aws.String(domain),
		StartRecordType: types.RRType(recordType),
		MaxItems:        aws.Int32(1),
	}

	result, err := d.client.ListResourceRecordSets(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list DNS records: %w", err)
	}

	var records []string
	for _, recordSet := range result.ResourceRecordSets {
		if *recordSet.Name == domain && string(recordSet.Type) == recordType {
			for _, rr := range recordSet.ResourceRecords {
				records = append(records, *rr.Value)
			}
			break
		}
	}

	return records, nil
}

// GetZoneName returns the DNS zone name
func (d *DNSService) GetZoneName() string {
	return d.zoneName
}

// findHostedZoneByName finds a hosted zone by domain name
func (d *DNSService) findHostedZoneByName(ctx context.Context, name string) (*types.HostedZone, error) {
	// Ensure name ends with a dot
	if !strings.HasSuffix(name, ".") {
		name = name + "."
	}

	input := &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(name),
		MaxItems: aws.Int32(100),
	}

	result, err := d.client.ListHostedZonesByName(ctx, input)
	if err != nil {
		return nil, err
	}

	for _, zone := range result.HostedZones {
		if *zone.Name == name {
			// Check if it's the right type (private vs public)
			if zone.Config != nil && zone.Config.PrivateZone == d.isPrivate {
				return &zone, nil
			}
		}
	}

	return nil, nil
}

// ensureHostedZone ensures the hosted zone is set
func (d *DNSService) ensureHostedZone(ctx context.Context) error {
	if d.hostedZoneID != "" {
		return nil
	}

	zone, err := d.findHostedZoneByName(ctx, d.zoneName)
	if err != nil {
		return fmt.Errorf("failed to find hosted zone: %w", err)
	}

	if zone == nil {
		return fmt.Errorf("hosted zone %s not found, please initialize DNS service first", d.zoneName)
	}

	d.hostedZoneID = *zone.Id
	return nil
}

// waitForChange waits for a Route53 change to be propagated
func (d *DNSService) waitForChange(ctx context.Context, changeID string) error {
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		input := &route53.GetChangeInput{
			Id: aws.String(changeID),
		}

		result, err := d.client.GetChange(ctx, input)
		if err != nil {
			return fmt.Errorf("failed to get change status: %w", err)
		}

		if result.ChangeInfo.Status == types.ChangeStatusInsync {
			return nil
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for DNS change to propagate")
}

// AssociateVPC associates an additional VPC with the hosted zone
func (d *DNSService) AssociateVPC(ctx context.Context, vpcID string, vpcRegion string) error {
	if d.hostedZoneID == "" {
		if err := d.ensureHostedZone(ctx); err != nil {
			return err
		}
	}

	input := &route53.AssociateVPCWithHostedZoneInput{
		HostedZoneId: aws.String(d.hostedZoneID),
		VPC: &types.VPC{
			VPCId:     aws.String(vpcID),
			VPCRegion: types.VPCRegion(vpcRegion),
		},
		Comment: aws.String("Associate VPC for Goman K3s cluster"),
	}

	_, err := d.client.AssociateVPCWithHostedZone(ctx, input)
	if err != nil {
		// Ignore if already associated
		if strings.Contains(err.Error(), "already associated") {
			log.Printf("[DNS] VPC %s already associated with hosted zone", vpcID)
			return nil
		}
		return fmt.Errorf("failed to associate VPC: %w", err)
	}

	log.Printf("[DNS] Associated VPC %s with hosted zone %s", vpcID, d.hostedZoneID)
	return nil
}