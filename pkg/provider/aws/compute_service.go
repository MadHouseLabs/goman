package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/madhouselabs/goman/pkg/logger"
	"github.com/madhouselabs/goman/pkg/provider"
	"github.com/madhouselabs/goman/pkg/utils"
)

// ComputeService implements compute operations using EC2
type ComputeService struct {
	client          *ec2.Client
	ssmClient       *ssm.Client
	iamClient       *iam.Client
	config          aws.Config
	instanceProfile string
	regionClients   map[string]*ec2.Client // Cache of region-specific EC2 clients
}

// NewComputeService creates a new EC2-based compute service
func NewComputeService(client *ec2.Client, iamClient *iam.Client, cfg aws.Config) *ComputeService {
	return &ComputeService{
		client:          client,
		ssmClient:       ssm.NewFromConfig(cfg),
		iamClient:       iamClient,
		config:          cfg,
		instanceProfile: "goman-ssm-instance-profile",
		regionClients:   make(map[string]*ec2.Client),
	}
}

// Initialize ensures the SSM instance profile exists
func (s *ComputeService) Initialize(ctx context.Context) error {
	// Ensure SSM instance profile exists
	if err := s.ensureSSMInstanceProfile(ctx); err != nil {
		return fmt.Errorf("failed to ensure SSM instance profile: %w", err)
	}
	return nil
}

// getEC2Client returns an EC2 client for the specified region
func (s *ComputeService) getEC2Client(region string) *ec2.Client {
	// If no region specified, use default client
	if region == "" {
		logger.Printf("Warning: No region specified, using default client for region: %s", s.config.Region)
		return s.client
	}

	// Check cache first
	if client, exists := s.regionClients[region]; exists {
		logger.Printf("Using cached EC2 client for region: %s", region)
		return client
	}

	// Create new client for the region
	logger.Printf("Creating new EC2 client for region: %s", region)
	cfg := s.config.Copy()
	cfg.Region = region
	client := ec2.NewFromConfig(cfg)
	s.regionClients[region] = client

	return client
}

// ensureSSMInstanceProfile creates the IAM instance profile for SSM if it doesn't exist
func (s *ComputeService) ensureSSMInstanceProfile(ctx context.Context) error {
	roleName := "goman-ssm-instance-role"
	profileName := s.instanceProfile

	// Check if role exists
	_, err := s.iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})

	if err != nil {
		// Create trust policy for EC2
		trustPolicy := map[string]interface{}{
			"Version": "2012-10-17",
			"Statement": []map[string]interface{}{
				{
					"Effect": "Allow",
					"Principal": map[string]string{
						"Service": "ec2.amazonaws.com",
					},
					"Action": "sts:AssumeRole",
				},
			},
		}

		trustPolicyJSON, err := json.Marshal(trustPolicy)
		if err != nil {
			return fmt.Errorf("failed to marshal trust policy: %w", err)
		}

		// Create the role
		_, err = s.iamClient.CreateRole(ctx, &iam.CreateRoleInput{
			RoleName:                 aws.String(roleName),
			AssumeRolePolicyDocument: aws.String(string(trustPolicyJSON)),
			Description:              aws.String("Role for goman EC2 instances to access SSM"),
			Tags: []iamTypes.Tag{
				{Key: aws.String("ManagedBy"), Value: aws.String("goman")},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create IAM role: %w", err)
		}

		// Attach SSM managed policy
		_, err = s.iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
			RoleName:  aws.String(roleName),
			PolicyArn: aws.String("arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"),
		})
		if err != nil {
			return fmt.Errorf("failed to attach SSM policy: %w", err)
		}
	}

	// Check if instance profile exists
	_, err = s.iamClient.GetInstanceProfile(ctx, &iam.GetInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
	})

	if err != nil {
		// Create instance profile
		_, err = s.iamClient.CreateInstanceProfile(ctx, &iam.CreateInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
			Tags: []iamTypes.Tag{
				{Key: aws.String("ManagedBy"), Value: aws.String("goman")},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create instance profile: %w", err)
		}

		// Add role to instance profile
		_, err = s.iamClient.AddRoleToInstanceProfile(ctx, &iam.AddRoleToInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
			RoleName:            aws.String(roleName),
		})
		if err != nil {
			return fmt.Errorf("failed to add role to instance profile: %w", err)
		}
	}

	return nil
}

// getLatestUbuntuAMI gets the latest Ubuntu 22.04 LTS AMI for the specified region
func (s *ComputeService) getLatestUbuntuAMI(ctx context.Context, region string) (string, error) {
	// Use SSM Parameter Store to get the latest Ubuntu 22.04 LTS AMI
	// AWS publishes these parameters in all regions
	ssmClient := ssm.NewFromConfig(s.config.Copy(), func(o *ssm.Options) {
		o.Region = region
	})

	// Parameter path for Ubuntu 22.04 LTS (Jammy) arm64/amd64
	parameterName := "/aws/service/canonical/ubuntu/server/22.04/stable/current/amd64/hvm/ebs-gp2/ami-id"

	result, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String(parameterName),
	})

	if err != nil {
		logger.Printf("Failed to get Ubuntu AMI from SSM for region %s: %v", region, err)
		// Fallback to Amazon Linux 2023 if Ubuntu parameter doesn't exist
		parameterName = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64"
		result, err = ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
			Name: aws.String(parameterName),
		})
		if err != nil {
			return "", fmt.Errorf("failed to get AMI from SSM Parameter Store: %w", err)
		}
	}

	amiID := aws.ToString(result.Parameter.Value)
	logger.Printf("Using AMI %s for region %s", amiID, region)
	return amiID, nil
}

// CreateInstance creates a new EC2 instance with retry logic
func (s *ComputeService) CreateInstance(ctx context.Context, config provider.InstanceConfig) (*provider.Instance, error) {
	// Log the target region for debugging
	logger.Printf("Creating instance %s in region: %s", config.Name, config.Region)

	// Get region-specific EC2 client
	ec2Client := s.getEC2Client(config.Region)

	// No SSH key needed - we'll use Systems Manager Session Manager instead
	// Convert tags to EC2 format
	var ec2Tags []types.Tag
	for k, v := range config.Tags {
		ec2Tags = append(ec2Tags, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	// Add name tag
	ec2Tags = append(ec2Tags, types.Tag{
		Key:   aws.String("Name"),
		Value: aws.String(config.Name),
	})

	// HARD RULE: Always ensure network infrastructure in the target region
	// This ensures we use the default VPC in the specified region
	networkInfo, err := s.ensureNetworkInfrastructure(ctx, config.Name, config.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure network infrastructure in region %s: %w", config.Region, err)
	}

	// Always use the network info from the target region
	config.SubnetID = networkInfo.SubnetID
	config.SecurityGroups = []string{networkInfo.SecurityGroupID}

	// Get region-specific AMI if not provided or if using default ap-south-1 AMI
	if config.ImageID == "" || config.ImageID == "ami-0f5ee92e2d63afc18" || strings.HasPrefix(config.ImageID, "ami-ubuntu") {
		amiID, err := s.getLatestUbuntuAMI(ctx, config.Region)
		if err != nil {
			return nil, fmt.Errorf("failed to get AMI for region %s: %w", config.Region, err)
		}
		config.ImageID = amiID
	}

	// Run instance with retry logic
	var result *ec2.RunInstancesOutput
	retryConfig := utils.DefaultRetryConfig()
	err = utils.RetryWithBackoff(ctx, retryConfig, func(ctx context.Context) error {
		var runErr error
		runInstancesInput := &ec2.RunInstancesInput{
			ImageId:      aws.String(config.ImageID),
			InstanceType: types.InstanceType(config.InstanceType),
			MinCount:     aws.Int32(1),
			MaxCount:     aws.Int32(1),
			// No KeyName - using Systems Manager instead
			SecurityGroupIds:      config.SecurityGroups,
			SubnetId:              aws.String(config.SubnetID),
			UserData:              aws.String(config.UserData),
			DisableApiTermination: aws.Bool(true), // Enable deletion protection
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeInstance,
					Tags:         ec2Tags,
				},
			},
		}

		// Add IAM instance profile for SSM access
		if config.InstanceProfile != "" {
			runInstancesInput.IamInstanceProfile = &types.IamInstanceProfileSpecification{
				Name: aws.String(config.InstanceProfile),
			}
		} else if s.instanceProfile != "" {
			// Use default SSM instance profile if not specified
			runInstancesInput.IamInstanceProfile = &types.IamInstanceProfileSpecification{
				Name: aws.String(s.instanceProfile),
			}
		}

		result, runErr = ec2Client.RunInstances(ctx, runInstancesInput)

		if runErr != nil && utils.IsRetryableError(runErr) {
			return runErr
		}
		return runErr
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create instance after retries: %w", err)
	}

	if len(result.Instances) == 0 {
		return nil, fmt.Errorf("no instances created")
	}

	inst := result.Instances[0]

	// Don't wait for instance to be running - return immediately
	// The reconciler will check the status in subsequent reconciliation loops
	// This allows parallel instance creation without blocking

	return s.convertToProviderInstance(&inst), nil
}

// DeleteInstance terminates an EC2 instance with retry logic
func (s *ComputeService) DeleteInstance(ctx context.Context, instanceID string) error {
	// Try to find which region the instance is in by checking our cached clients
	ec2Client := s.client // Default to main client

	// Try to find the instance in cached regions
	for region, client := range s.regionClients {
		result, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: []string{instanceID},
		})
		if err == nil && len(result.Reservations) > 0 && len(result.Reservations[0].Instances) > 0 {
			ec2Client = client
			logger.Printf("Found instance %s in region %s", instanceID, region)
			break
		}
	}

	// If not found in cached regions, try with default client
	if ec2Client == s.client {
		// Check if instance exists in default region
		result, err := s.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: []string{instanceID},
		})
		if err != nil || len(result.Reservations) == 0 {
			logger.Printf("Warning: Instance %s not found in any known region, attempting deletion with default client", instanceID)
		}
	}

	// First, disable deletion protection
	_, err := ec2Client.ModifyInstanceAttribute(ctx, &ec2.ModifyInstanceAttributeInput{
		InstanceId: aws.String(instanceID),
		DisableApiTermination: &types.AttributeBooleanValue{
			Value: aws.Bool(false),
		},
	})
	if err != nil {
		logger.Printf("Warning: Failed to disable deletion protection for %s: %v", instanceID, err)
		// Continue anyway - the instance might not have protection enabled
	}

	// Now terminate the instance
	retryConfig := utils.DefaultRetryConfig()
	err = utils.RetryWithBackoff(ctx, retryConfig, func(ctx context.Context) error {
		_, err := ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
			InstanceIds: []string{instanceID},
		})

		if err != nil && utils.IsRetryableError(err) {
			return err
		}
		return err
	})

	if err != nil {
		return fmt.Errorf("failed to terminate instance after retries: %w", err)
	}

	// Note: Cleanup of associated resources (security groups, key pairs) should be done
	// by the controller after all instances are deleted, not here

	return nil
}

// GetInstance gets instance details with retry logic
func (s *ComputeService) GetInstance(ctx context.Context, instanceID string) (*provider.Instance, error) {
	var result *ec2.DescribeInstancesOutput
	retryConfig := utils.DefaultRetryConfig()
	err := utils.RetryWithBackoff(ctx, retryConfig, func(ctx context.Context) error {
		var descErr error
		result, descErr = s.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: []string{instanceID},
		})

		if descErr != nil && utils.IsRetryableError(descErr) {
			return descErr
		}
		return descErr
	})

	if err != nil {
		return nil, fmt.Errorf("failed to describe instance after retries: %w", err)
	}

	if len(result.Reservations) == 0 || len(result.Reservations[0].Instances) == 0 {
		return nil, fmt.Errorf("instance not found")
	}

	return s.convertToProviderInstance(&result.Reservations[0].Instances[0]), nil
}

// ListInstances lists instances with filters
func (s *ComputeService) ListInstances(ctx context.Context, filters map[string]string) ([]*provider.Instance, error) {
	// Check if a specific region is requested via special filter
	region := ""
	if r, ok := filters["region"]; ok {
		region = r
		delete(filters, "region") // Remove from filters as it's not an EC2 filter
	}

	// Get the appropriate EC2 client
	var ec2Client *ec2.Client
	if region != "" {
		ec2Client = s.getEC2Client(region)
		logger.Printf("Listing instances in specific region: %s", region)
	} else {
		// If no region specified, we should check all regions where we have instances
		// For now, use default client but log a warning
		ec2Client = s.client
		logger.Printf("Warning: ListInstances using default region. May miss instances in other regions.")
	}

	// Convert filters to EC2 format
	var ec2Filters []types.Filter
	for k, v := range filters {
		// Handle comma-separated values for filters like instance-state-name
		values := []string{v}
		if strings.Contains(v, ",") {
			values = strings.Split(v, ",")
			// Trim spaces from each value
			for i, val := range values {
				values[i] = strings.TrimSpace(val)
			}
		}
		ec2Filters = append(ec2Filters, types.Filter{
			Name:   aws.String(k),
			Values: values,
		})
	}

	result, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: ec2Filters,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}

	var instances []*provider.Instance
	for _, reservation := range result.Reservations {
		for _, inst := range reservation.Instances {
			instances = append(instances, s.convertToProviderInstance(&inst))
		}
	}

	return instances, nil
}

// StartInstance starts a stopped instance
func (s *ComputeService) StartInstance(ctx context.Context, instanceID string) error {
	_, err := s.client.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []string{instanceID},
	})

	if err != nil {
		return fmt.Errorf("failed to start instance: %w", err)
	}

	return nil
}

// StopInstance stops a running instance
func (s *ComputeService) StopInstance(ctx context.Context, instanceID string) error {
	_, err := s.client.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []string{instanceID},
	})

	if err != nil {
		return fmt.Errorf("failed to stop instance: %w", err)
	}

	return nil
}

// convertToProviderInstance converts EC2 instance to provider instance
func (s *ComputeService) convertToProviderInstance(inst *types.Instance) *provider.Instance {
	p := &provider.Instance{
		ID:           aws.ToString(inst.InstanceId),
		State:        string(inst.State.Name),
		InstanceType: string(inst.InstanceType),
		Tags:         make(map[string]string),
	}

	if inst.PrivateIpAddress != nil {
		p.PrivateIP = *inst.PrivateIpAddress
	}

	if inst.PublicIpAddress != nil {
		p.PublicIP = *inst.PublicIpAddress
	}

	if inst.LaunchTime != nil {
		p.LaunchTime = *inst.LaunchTime
	}

	// Extract tags
	for _, tag := range inst.Tags {
		if tag.Key != nil && tag.Value != nil {
			p.Tags[*tag.Key] = *tag.Value
			if *tag.Key == "Name" {
				p.Name = *tag.Value
			}
		}
	}

	return p
}

// SSH key pairs are no longer needed - we use Systems Manager for remote access

// NetworkInfo holds network infrastructure details
type NetworkInfo struct {
	VPCID           string
	SubnetID        string
	SecurityGroupID string
}

// ensureNetworkInfrastructure ensures VPC, subnet, and security group exist in the specified region
func (s *ComputeService) ensureNetworkInfrastructure(ctx context.Context, resourceName string, region string) (*NetworkInfo, error) {
	// Use the default VPC and subnets for simplicity
	// These resources are reused across all clusters

	logger.Printf("Ensuring network infrastructure for %s in region: %s", resourceName, region)

	// Get region-specific EC2 client
	ec2Client := s.getEC2Client(region)

	// Get default VPC
	describeVpcsOutput, err := ec2Client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("is-default"),
				Values: []string{"true"},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe VPCs: %w", err)
	}

	if len(describeVpcsOutput.Vpcs) == 0 {
		return nil, fmt.Errorf("no default VPC found")
	}

	vpcID := aws.ToString(describeVpcsOutput.Vpcs[0].VpcId)

	// Get default subnet in the first AZ
	describeSubnetsOutput, err := ec2Client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []string{vpcID},
			},
			{
				Name:   aws.String("default-for-az"),
				Values: []string{"true"},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe subnets: %w", err)
	}

	if len(describeSubnetsOutput.Subnets) == 0 {
		return nil, fmt.Errorf("no default subnets found")
	}

	subnetID := aws.ToString(describeSubnetsOutput.Subnets[0].SubnetId)

	// Extract cluster name from resource name 
	// Formats: {cluster}-master-{index} or {cluster}-worker-{index}
	clusterName := resourceName
	
	// Try to extract cluster name by looking for -master- or -worker-
	if idx := strings.LastIndex(resourceName, "-master-"); idx > 0 {
		clusterName = resourceName[:idx]
	} else if idx := strings.LastIndex(resourceName, "-worker-"); idx > 0 {
		clusterName = resourceName[:idx]
	}

	// Create or get security group
	sgName := fmt.Sprintf("goman-%s-sg", clusterName)

	// Check if security group exists
	describeSGOutput, err := ec2Client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("group-name"),
				Values: []string{sgName},
			},
			{
				Name:   aws.String("vpc-id"),
				Values: []string{vpcID},
			},
		},
	})

	var securityGroupID string
	if err != nil || len(describeSGOutput.SecurityGroups) == 0 {
		// Create security group (will be reused if cluster is recreated)
		logger.Printf("Creating new security group %s for cluster %s", sgName, clusterName)
		createSGOutput, err := ec2Client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
			GroupName:   aws.String(sgName),
			Description: aws.String(fmt.Sprintf("Security group for goman cluster %s (reusable)", clusterName)),
			VpcId:       aws.String(vpcID),
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeSecurityGroup,
					Tags: []types.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String(sgName),
						},
						{
							Key:   aws.String("ManagedBy"),
							Value: aws.String("goman"),
						},
						{
							Key:   aws.String("Cluster"),
							Value: aws.String(clusterName),
						},
					},
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create security group: %w", err)
		}

		securityGroupID = aws.ToString(createSGOutput.GroupId)

		// Add ingress rules for K3s
		_, err = ec2Client.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
			GroupId: aws.String(securityGroupID),
			IpPermissions: []types.IpPermission{
				// No SSH access needed - using Systems Manager Session Manager
				// K3s API server - restricted to VPC CIDR
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int32(6443),
					ToPort:     aws.Int32(6443),
					UserIdGroupPairs: []types.UserIdGroupPair{
						{
							GroupId:     aws.String(securityGroupID),
							Description: aws.String("K3s API server - internal access only"),
						},
					},
				},
				// K3s node communication
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int32(10250),
					ToPort:     aws.Int32(10250),
					UserIdGroupPairs: []types.UserIdGroupPair{
						{
							GroupId:     aws.String(securityGroupID),
							Description: aws.String("Kubelet API"),
						},
					},
				},
				// Flannel VXLAN
				{
					IpProtocol: aws.String("udp"),
					FromPort:   aws.Int32(8472),
					ToPort:     aws.Int32(8472),
					UserIdGroupPairs: []types.UserIdGroupPair{
						{
							GroupId:     aws.String(securityGroupID),
							Description: aws.String("Flannel VXLAN"),
						},
					},
				},
			},
		})
		if err != nil {
			logger.Printf("Warning: failed to add ingress rules: %v", err)
		}
	} else {
		// Reuse existing security group
		securityGroupID = aws.ToString(describeSGOutput.SecurityGroups[0].GroupId)
		logger.Printf("Reusing existing security group %s (ID: %s)", sgName, securityGroupID)
	}

	return &NetworkInfo{
		VPCID:           vpcID,
		SubnetID:        subnetID,
		SecurityGroupID: securityGroupID,
	}, nil
}

// RunCommand executes a command on instances using AWS Systems Manager
func (s *ComputeService) RunCommand(ctx context.Context, instanceIDs []string, command string) (*provider.CommandResult, error) {
	// Check if SSM client is initialized
	if s.ssmClient == nil {
		return nil, fmt.Errorf("SSM client not initialized - Systems Manager support not available")
	}

	// Send command to instances
	result, err := s.ssmClient.SendCommand(ctx, &ssm.SendCommandInput{
		InstanceIds:  instanceIDs,
		DocumentName: aws.String("AWS-RunShellScript"),
		Parameters: map[string][]string{
			"commands": {command},
		},
		TimeoutSeconds: aws.Int32(300), // 5 minutes timeout
	})

	if err != nil {
		return nil, fmt.Errorf("failed to send command: %w", err)
	}

	commandID := aws.ToString(result.Command.CommandId)

	// Wait for command to complete (with timeout)
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Poll for command completion
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("timeout waiting for command to complete")
		case <-ticker.C:
			// Get command invocations
			invocations, err := s.ssmClient.ListCommandInvocations(waitCtx, &ssm.ListCommandInvocationsInput{
				CommandId: aws.String(commandID),
			})

			if err != nil {
				return nil, fmt.Errorf("failed to get command status: %w", err)
			}

			// Check if all invocations are complete
			allComplete := true
			cmdResult := &provider.CommandResult{
				CommandID: commandID,
				Status:    "Success",
				Instances: make(map[string]*provider.InstanceCommandResult),
			}

			for _, inv := range invocations.CommandInvocations {
				instanceID := aws.ToString(inv.InstanceId)
				status := string(inv.Status)

				if status == "InProgress" || status == "Pending" {
					allComplete = false
					continue
				}

				// Get command output
				output, err := s.ssmClient.GetCommandInvocation(waitCtx, &ssm.GetCommandInvocationInput{
					CommandId:  aws.String(commandID),
					InstanceId: aws.String(instanceID),
				})

				instanceResult := &provider.InstanceCommandResult{
					InstanceID: instanceID,
					Status:     status,
				}

				if err == nil {
					instanceResult.Output = aws.ToString(output.StandardOutputContent)
					instanceResult.Error = aws.ToString(output.StandardErrorContent)
					instanceResult.ExitCode = int(output.ResponseCode)
				}

				cmdResult.Instances[instanceID] = instanceResult

				if status != "Success" {
					cmdResult.Status = "Failed"
				}
			}

			if allComplete {
				return cmdResult, nil
			}
		}
	}
}
