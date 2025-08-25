package aws

import (
	"context"
	"encoding/base64"
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
	accountID       string
	regionClients    map[string]*ec2.Client // Cache of region-specific EC2 clients
	regionSSMClients map[string]*ssm.Client // Cache of region-specific SSM clients
}

// NewComputeService creates a new EC2-based compute service
func NewComputeService(client *ec2.Client, iamClient *iam.Client, cfg aws.Config, accountID string) *ComputeService {
	return &ComputeService{
		client:          client,
		ssmClient:       ssm.NewFromConfig(cfg),
		iamClient:       iamClient,
		config:          cfg,
		instanceProfile: "goman-ssm-instance-profile",
		accountID:       accountID,
		regionClients:    make(map[string]*ec2.Client),
		regionSSMClients: make(map[string]*ssm.Client),
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

// detectInstanceRegion detects which region an instance is in
func (s *ComputeService) detectInstanceRegion(ctx context.Context, instanceID string) string {
	// First try the default region (most likely case)
	result, err := s.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err == nil && len(result.Reservations) > 0 && len(result.Reservations[0].Instances) > 0 {
		logger.Printf("Found instance %s in default region %s", instanceID, s.config.Region)
		return s.config.Region
	}
	
	// Then check cached region clients
	for region, client := range s.regionClients {
		result, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: []string{instanceID},
		})
		if err == nil && len(result.Reservations) > 0 && len(result.Reservations[0].Instances) > 0 {
			logger.Printf("Found instance %s in cached region %s", instanceID, region)
			return region
		}
	}
	
	logger.Printf("Could not detect region for instance %s", instanceID)
	return ""
}

// getSSMClient returns an SSM client for the specified region
func (s *ComputeService) getSSMClient(region string) *ssm.Client {
	// If no region specified, use default client
	if region == "" {
		logger.Printf("Warning: No region specified, using default SSM client for region: %s", s.config.Region)
		return s.ssmClient
	}

	// Check cache first
	if client, exists := s.regionSSMClients[region]; exists {
		logger.Printf("Using cached SSM client for region: %s", region)
		return client
	}

	// Create new client for the region
	logger.Printf("Creating new SSM client for region: %s", region)
	cfg := s.config.Copy()
	cfg.Region = region
	client := ssm.NewFromConfig(cfg)
	s.regionSSMClients[region] = client

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

		// Create and attach custom policy for S3 access to K3s binaries
		policyName := fmt.Sprintf("goman-instance-s3-policy-%s", s.accountID)
		bucketName := fmt.Sprintf("goman-%s", s.accountID)
		
		// Create the policy document for S3 read access
		policyDoc := map[string]interface{}{
			"Version": "2012-10-17",
			"Statement": []map[string]interface{}{
				{
					"Effect": "Allow",
					"Action": []string{
						"s3:GetObject",
						"s3:ListBucket",
					},
					"Resource": []string{
						fmt.Sprintf("arn:aws:s3:::%s/*", bucketName),
						fmt.Sprintf("arn:aws:s3:::%s", bucketName),
					},
				},
			},
		}

		policyJSON, err := json.Marshal(policyDoc)
		if err != nil {
			return fmt.Errorf("failed to marshal S3 policy: %w", err)
		}

		// Try to create the policy
		_, err = s.iamClient.CreatePolicy(ctx, &iam.CreatePolicyInput{
			PolicyName:     aws.String(policyName),
			PolicyDocument: aws.String(string(policyJSON)),
			Description:    aws.String("Policy for goman instances to access K3s binaries in S3"),
		})
		if err != nil {
			// Policy might already exist, that's okay
			if !strings.Contains(err.Error(), "EntityAlreadyExists") {
				logger.Printf("Warning: Failed to create S3 policy: %v", err)
			}
		}

		// Attach the custom S3 policy
		_, err = s.iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
			RoleName:  aws.String(roleName),
			PolicyArn: aws.String(fmt.Sprintf("arn:aws:iam::%s:policy/%s", s.accountID, policyName)),
		})
		if err != nil {
			// Don't fail if policy attachment fails, instances can still work without S3 access
			logger.Printf("Warning: Failed to attach S3 policy: %v (instances will fallback to GitHub downloads)", err)
		}
	}

	// Check if instance profile exists
	profileResp, err := s.iamClient.GetInstanceProfile(ctx, &iam.GetInstanceProfileInput{
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
	} else {
		// Instance profile exists, check if role is attached
		hasRole := false
		for _, role := range profileResp.InstanceProfile.Roles {
			if aws.ToString(role.RoleName) == roleName {
				hasRole = true
				break
			}
		}
		
		if !hasRole {
			// Add role to existing instance profile
			_, err = s.iamClient.AddRoleToInstanceProfile(ctx, &iam.AddRoleToInstanceProfileInput{
				InstanceProfileName: aws.String(profileName),
				RoleName:            aws.String(roleName),
			})
			if err != nil && !strings.Contains(err.Error(), "LimitExceeded") {
				return fmt.Errorf("failed to add role to instance profile: %w", err)
			}
		}
	}

	return nil
}

// getLatestAmazonLinux2AMI gets the latest Amazon Linux 2 AMI for the specified region
func (s *ComputeService) getLatestAmazonLinux2AMI(ctx context.Context, region string) (string, error) {
	// Use SSM Parameter Store to get the latest Amazon Linux 2 AMI
	// AWS publishes these parameters in all regions
	ssmClient := ssm.NewFromConfig(s.config.Copy(), func(o *ssm.Options) {
		o.Region = region
	})

	// Parameter path for Amazon Linux 2 (has SSM agent pre-installed and configured)
	parameterName := "/aws/service/ami-amazon-linux-latest/amzn2-ami-hvm-x86_64-gp2"

	result, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String(parameterName),
	})

	if err != nil {
		logger.Printf("Failed to get Amazon Linux 2 AMI from SSM for region %s: %v", region, err)
		// Fallback to Ubuntu if Amazon Linux 2 parameter doesn't exist
		parameterName = "/aws/service/canonical/ubuntu/server/22.04/stable/current/amd64/hvm/ebs-gp2/ami-id"
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

	// Always use Amazon Linux 2 AMI for AWS (provider-specific decision)
	amiID, err := s.getLatestAmazonLinux2AMI(ctx, config.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to get AMI for region %s: %w", config.Region, err)
	}
	config.ImageID = amiID
	
	// AWS-specific: Always use SSM instance profile
	if config.InstanceProfile == "" {
		config.InstanceProfile = s.instanceProfile
	}
	
	// AWS-specific: Add UserData to install K3s and ensure SSM agent is running
	if config.UserData == "" {
		// Extract cluster name and role from tags
		clusterName := config.Tags["goman-cluster"]
		role := config.Tags["goman-role"]
		nodeIndex := config.Tags["goman-index"]
		masterIP := config.Tags["goman-master-ip"] // For additional HA masters and workers
		nodeToken := config.Tags["goman-node-token"] // For workers to join cluster
		
		// Build the user data script based on role
		userDataScript := fmt.Sprintf(`#!/bin/bash
set -e

# Log startup
echo "[$(date)] Starting instance initialization" >> /var/log/goman-startup.log

# Amazon Linux 2 has SSM agent pre-installed
# Just ensure it's enabled and running
systemctl enable amazon-ssm-agent
systemctl start amazon-ssm-agent

# Wait for SSM agent to be ready
sleep 10

# Set up environment
export CLUSTER_NAME="%s"
export NODE_ROLE="%s"
export AWS_REGION="%s"
export S3_BUCKET="goman-%s"
export NODE_INDEX="%s"
export MASTER_IP="%s"
export NODE_TOKEN="%s"

echo "[$(date)] Cluster: $CLUSTER_NAME, Role: $NODE_ROLE, Index: $NODE_INDEX" >> /var/log/goman-startup.log

# Install required packages
yum update -y
yum install -y jq

# Download K3s binary from S3
echo "[$(date)] Downloading K3s binary from S3..." >> /var/log/goman-startup.log
# Use a specific version for now (can be made configurable later)
K3S_VERSION="v1.31.4+k3s1"
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    ARCH="amd64"
elif [ "$ARCH" = "aarch64" ]; then
    ARCH="arm64"
fi

aws s3 cp s3://$S3_BUCKET/binaries/k3s/$K3S_VERSION/k3s-$ARCH /usr/local/bin/k3s
if [ $? -ne 0 ]; then
    echo "[$(date)] ERROR: Failed to download K3s binary from S3" >> /var/log/goman-startup.log
    exit 1
fi
chmod +x /usr/local/bin/k3s

# Create symlinks for kubectl and other tools
ln -sf /usr/local/bin/k3s /usr/local/bin/kubectl
ln -sf /usr/local/bin/k3s /usr/local/bin/crictl
ln -sf /usr/local/bin/k3s /usr/local/bin/ctr

# Get tokens from S3
if [ "$NODE_ROLE" = "master" ]; then
    # Get server token from S3
    SERVER_TOKEN=$(aws s3 cp s3://$S3_BUCKET/clusters/$CLUSTER_NAME/k3s-server-token - 2>/dev/null || echo "")
    
    if [ -z "$SERVER_TOKEN" ]; then
        echo "[$(date)] ERROR: Failed to get server token from S3" >> /var/log/goman-startup.log
        exit 1
    fi
    
    # Get instance private IP
    PRIVATE_IP=$(curl -s http://169.254.169.254/latest/meta-data/local-ipv4)
    
    # Determine if this is the first master or additional HA master
    if [ "$NODE_INDEX" = "0" ] || [ -z "$MASTER_IP" ]; then
        # First master - initialize new cluster
        echo "[$(date)] Installing K3s server as first master..." >> /var/log/goman-startup.log
        
        # Check if this is HA mode by looking for a specific tag or checking node count
        # For HA mode, we need --cluster-init to enable embedded etcd
        CLUSTER_INIT_FLAG=""
        if [ -n "$NODE_INDEX" ] && [ "$NODE_INDEX" = "0" ]; then
            # This is explicitly the first master in HA mode
            CLUSTER_INIT_FLAG="--cluster-init"
            echo "[$(date)] Enabling embedded etcd for HA mode" >> /var/log/goman-startup.log
        fi
        
        # Create K3s systemd service for first master
        cat > /etc/systemd/system/k3s.service <<EOF
[Unit]
Description=Lightweight Kubernetes
Documentation=https://k3s.io
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
EnvironmentFile=-/etc/systemd/system/k3s.service.env
KillMode=process
Delegate=yes
LimitNOFILE=1048576
LimitNPROC=infinity
LimitCORE=infinity
TasksMax=infinity
TimeoutStartSec=0
Restart=always
RestartSec=5s
ExecStartPre=/bin/sh -xc '! /usr/bin/systemctl is-enabled --quiet nm-cloud-setup.service'
ExecStartPre=-/sbin/modprobe br_netfilter
ExecStartPre=-/sbin/modprobe overlay
ExecStart=/usr/local/bin/k3s server ${CLUSTER_INIT_FLAG} --token=${SERVER_TOKEN} --node-ip=${PRIVATE_IP} --flannel-iface=eth0 --disable=traefik --disable=servicelb --disable=metrics-server --write-kubeconfig-mode=644

[Install]
WantedBy=multi-user.target
EOF

    # Create environment file with actual values
    cat > /etc/systemd/system/k3s.service.env <<EOF
SERVER_TOKEN=${SERVER_TOKEN}
PRIVATE_IP=${PRIVATE_IP}
CLUSTER_INIT_FLAG=${CLUSTER_INIT_FLAG}
EOF

        # Start K3s server
        systemctl daemon-reload
        systemctl enable k3s.service
        systemctl start k3s.service
        
        # Wait for K3s to be ready
        echo "[$(date)] Waiting for K3s to be ready..." >> /var/log/goman-startup.log
        for i in {1..60}; do
            if kubectl get nodes >/dev/null 2>&1; then
                echo "[$(date)] K3s is ready!" >> /var/log/goman-startup.log
                break
            fi
            sleep 5
        done
        
        # Save kubeconfig to S3
        if [ -f /etc/rancher/k3s/k3s.yaml ]; then
            # Replace localhost with instance public IP
            PUBLIC_IP=$(curl -s http://169.254.169.254/latest/meta-data/public-ipv4)
            sed "s/127.0.0.1/$PUBLIC_IP/g" /etc/rancher/k3s/k3s.yaml > /tmp/kubeconfig.yaml
            aws s3 cp /tmp/kubeconfig.yaml s3://$S3_BUCKET/clusters/$CLUSTER_NAME/kubeconfig.yaml
            echo "[$(date)] Kubeconfig saved to S3" >> /var/log/goman-startup.log
        fi
        
    else
        # Additional HA master - join existing cluster
        echo "[$(date)] Installing K3s server as additional HA master, joining $MASTER_IP..." >> /var/log/goman-startup.log
        
        # Wait a bit for the first master to be fully ready
        echo "[$(date)] Waiting 30 seconds for first master to initialize etcd..." >> /var/log/goman-startup.log
        sleep 30
        
        # Create K3s systemd service for additional master
        cat > /etc/systemd/system/k3s.service <<EOF
[Unit]
Description=Lightweight Kubernetes
Documentation=https://k3s.io
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
EnvironmentFile=-/etc/systemd/system/k3s.service.env
KillMode=process
Delegate=yes
LimitNOFILE=1048576
LimitNPROC=infinity
LimitCORE=infinity
TasksMax=infinity
TimeoutStartSec=0
Restart=always
RestartSec=5s
ExecStartPre=/bin/sh -xc '! /usr/bin/systemctl is-enabled --quiet nm-cloud-setup.service'
ExecStartPre=-/sbin/modprobe br_netfilter
ExecStartPre=-/sbin/modprobe overlay
ExecStart=/usr/local/bin/k3s server --server=https://${MASTER_IP}:6443 --token=${SERVER_TOKEN} --node-ip=${PRIVATE_IP} --flannel-iface=eth0 --disable=traefik --disable=servicelb --disable=metrics-server --write-kubeconfig-mode=644

[Install]
WantedBy=multi-user.target
EOF

        # Create environment file with actual values
        cat > /etc/systemd/system/k3s.service.env <<EOF
SERVER_TOKEN=${SERVER_TOKEN}
PRIVATE_IP=${PRIVATE_IP}
MASTER_IP=${MASTER_IP}
EOF

        # Start K3s server
        systemctl daemon-reload
        systemctl enable k3s.service
        systemctl start k3s.service
        
        echo "[$(date)] K3s HA master installation initiated, joining cluster at $MASTER_IP" >> /var/log/goman-startup.log
    fi
    
elif [ "$NODE_ROLE" = "worker" ]; then
    # NODE_TOKEN and MASTER_IP are already set from environment variables
    # They come from the EC2 tags passed to the instance
    
    if [ -z "$NODE_TOKEN" ]; then
        echo "[$(date)] ERROR: Node token not provided" >> /var/log/goman-startup.log
        exit 1
    fi
    
    if [ -z "$MASTER_IP" ]; then
        echo "[$(date)] ERROR: Master IP not configured" >> /var/log/goman-startup.log
        exit 1
    fi
    
    echo "[$(date)] Installing K3s agent to join cluster at $MASTER_IP" >> /var/log/goman-startup.log
    
    # Create K3s agent systemd service
    cat > /etc/systemd/system/k3s-agent.service <<EOF
[Unit]
Description=Lightweight Kubernetes Agent
Documentation=https://k3s.io
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
EnvironmentFile=-/etc/systemd/system/k3s-agent.service.env
KillMode=process
Delegate=yes
LimitNOFILE=1048576
LimitNPROC=infinity
LimitCORE=infinity
TasksMax=infinity
TimeoutStartSec=0
Restart=always
RestartSec=5s
ExecStartPre=/bin/sh -xc '! /usr/bin/systemctl is-enabled --quiet nm-cloud-setup.service'
ExecStartPre=-/sbin/modprobe br_netfilter
ExecStartPre=-/sbin/modprobe overlay
ExecStart=/usr/local/bin/k3s agent --server=https://${MASTER_IP}:6443 --token=${NODE_TOKEN}

[Install]
WantedBy=multi-user.target
EOF

    # Create environment file with actual values
    cat > /etc/systemd/system/k3s-agent.service.env <<EOF
MASTER_IP=${MASTER_IP}
NODE_TOKEN=${NODE_TOKEN}
EOF
    
    # Start K3s agent
    systemctl daemon-reload
    systemctl enable k3s-agent.service
    systemctl start k3s-agent.service
    
    echo "[$(date)] K3s agent installation initiated" >> /var/log/goman-startup.log
fi

echo "[$(date)] K3s installation completed" >> /var/log/goman-startup.log

# Log for debugging
echo "Instance started at $(date)" >> /var/log/instance-startup.log
`, clusterName, role, config.Region, s.accountID, nodeIndex, masterIP, nodeToken)
		
		config.UserData = base64.StdEncoding.EncodeToString([]byte(userDataScript))
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

		// Add IAM instance profile for SSM access (always set by now)
		runInstancesInput.IamInstanceProfile = &types.IamInstanceProfileSpecification{
			Name: aws.String(config.InstanceProfile),
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

	// Detect instance region
	instanceRegion := s.detectInstanceRegion(ctx, instanceID)
	if instanceRegion != "" && instanceRegion != s.config.Region {
		ec2Client = s.getEC2Client(instanceRegion)
		logger.Printf("Using region-specific client for instance %s in %s", instanceID, instanceRegion)
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

// ModifyInstanceType changes the instance type of a stopped instance
func (s *ComputeService) ModifyInstanceType(ctx context.Context, instanceID string, instanceType string) error {
	// Try to find which region the instance is in
	ec2Client := s.client

	// Detect instance region
	instanceRegion := s.detectInstanceRegion(ctx, instanceID)
	if instanceRegion != "" && instanceRegion != s.config.Region {
		ec2Client = s.getEC2Client(instanceRegion)
		logger.Printf("Using region-specific client for instance %s modification in %s", instanceID, instanceRegion)
	}

	// Modify the instance attribute
	_, err := ec2Client.ModifyInstanceAttribute(ctx, &ec2.ModifyInstanceAttributeInput{
		InstanceId: aws.String(instanceID),
		InstanceType: &types.AttributeValue{
			Value: aws.String(instanceType),
		},
	})

	if err != nil {
		return fmt.Errorf("failed to modify instance type: %w", err)
	}

	logger.Printf("Successfully modified instance %s to type %s", instanceID, instanceType)
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
				// K3s API server - allow from all nodes
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int32(6443),
					ToPort:     aws.Int32(6443),
					UserIdGroupPairs: []types.UserIdGroupPair{
						{
							GroupId:     aws.String(securityGroupID),
							Description: aws.String("K3s API server - all nodes"),
						},
					},
				},
				// etcd client/server communication - CRITICAL for HA clusters
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int32(2379),
					ToPort:     aws.Int32(2380),
					UserIdGroupPairs: []types.UserIdGroupPair{
						{
							GroupId:     aws.String(securityGroupID),
							Description: aws.String("etcd client and peer - required for HA"),
						},
					},
				},
				// Kubelet metrics
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int32(10250),
					ToPort:     aws.Int32(10250),
					UserIdGroupPairs: []types.UserIdGroupPair{
						{
							GroupId:     aws.String(securityGroupID),
							Description: aws.String("Kubelet metrics"),
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
				// Flannel Wireguard with IPv4 (optional)
				{
					IpProtocol: aws.String("udp"),
					FromPort:   aws.Int32(51820),
					ToPort:     aws.Int32(51820),
					UserIdGroupPairs: []types.UserIdGroupPair{
						{
							GroupId:     aws.String(securityGroupID),
							Description: aws.String("Flannel Wireguard IPv4"),
						},
					},
				},
				// Flannel Wireguard with IPv6 (optional)
				{
					IpProtocol: aws.String("udp"),
					FromPort:   aws.Int32(51821),
					ToPort:     aws.Int32(51821),
					UserIdGroupPairs: []types.UserIdGroupPair{
						{
							GroupId:     aws.String(securityGroupID),
							Description: aws.String("Flannel Wireguard IPv6"),
						},
					},
				},
				// Embedded distributed registry - Spegel (optional)
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int32(5001),
					ToPort:     aws.Int32(5001),
					UserIdGroupPairs: []types.UserIdGroupPair{
						{
							GroupId:     aws.String(securityGroupID),
							Description: aws.String("Spegel distributed registry"),
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
	if len(instanceIDs) == 0 {
		return nil, fmt.Errorf("no instance IDs provided")
	}

	// Detect the region of the first instance
	// Assume all instances in the same command are in the same region
	instanceRegion := s.detectInstanceRegion(ctx, instanceIDs[0])

	// Get the appropriate SSM client for the region
	var ssmClient *ssm.Client
	if instanceRegion != "" {
		ssmClient = s.getSSMClient(instanceRegion)
	} else {
		// Fallback to default SSM client
		logger.Printf("Could not detect region for instance %s, using default SSM client", instanceIDs[0])
		ssmClient = s.ssmClient
	}

	// Check if SSM client is initialized
	if ssmClient == nil {
		return nil, fmt.Errorf("SSM client not initialized - Systems Manager support not available")
	}

	// Send command to instances
	result, err := ssmClient.SendCommand(ctx, &ssm.SendCommandInput{
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
			invocations, err := ssmClient.ListCommandInvocations(waitCtx, &ssm.ListCommandInvocationsInput{
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
				output, err := ssmClient.GetCommandInvocation(waitCtx, &ssm.GetCommandInvocationInput{
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

// StartCommand starts a command on instances without waiting for completion (non-blocking)
func (s *ComputeService) StartCommand(ctx context.Context, instanceIDs []string, command string) (string, error) {
	if len(instanceIDs) == 0 {
		return "", fmt.Errorf("no instance IDs provided")
	}

	// Detect the region of the first instance
	instanceRegion := s.detectInstanceRegion(ctx, instanceIDs[0])

	// Get the appropriate SSM client for the region
	var ssmClient *ssm.Client
	if instanceRegion != "" {
		ssmClient = s.getSSMClient(instanceRegion)
	} else {
		logger.Printf("Could not detect region for instance %s, using default SSM client", instanceIDs[0])
		ssmClient = s.ssmClient
	}

	// Check if SSM client is initialized
	if ssmClient == nil {
		return "", fmt.Errorf("SSM client not initialized - Systems Manager support not available")
	}

	// Send command to instances
	result, err := ssmClient.SendCommand(ctx, &ssm.SendCommandInput{
		InstanceIds:  instanceIDs,
		DocumentName: aws.String("AWS-RunShellScript"),
		Parameters: map[string][]string{
			"commands": {command},
		},
		TimeoutSeconds: aws.Int32(600), // 10 minutes timeout (increased)
	})

	if err != nil {
		return "", fmt.Errorf("failed to send command: %w", err)
	}

	commandID := aws.ToString(result.Command.CommandId)
	logger.Printf("Started command %s on instances %v", commandID, instanceIDs)
	
	return commandID, nil
}

// GetCommandResult checks the status of a previously started command
func (s *ComputeService) GetCommandResult(ctx context.Context, commandID string) (*provider.CommandResult, error) {
	if commandID == "" {
		return nil, fmt.Errorf("command ID cannot be empty")
	}

	// Try all SSM clients to find the command (it could be in any region)
	var ssmClient *ssm.Client
	var invocations *ssm.ListCommandInvocationsOutput
	var err error

	// First try the default client
	if s.ssmClient != nil {
		invocations, err = s.ssmClient.ListCommandInvocations(ctx, &ssm.ListCommandInvocationsInput{
			CommandId: aws.String(commandID),
		})
		if err == nil && len(invocations.CommandInvocations) > 0 {
			ssmClient = s.ssmClient
		}
	}

	// If not found, try other region clients
	if ssmClient == nil {
		for _, client := range s.regionSSMClients {
			invocations, err = client.ListCommandInvocations(ctx, &ssm.ListCommandInvocationsInput{
				CommandId: aws.String(commandID),
			})
			if err == nil && len(invocations.CommandInvocations) > 0 {
				ssmClient = client
				break
			}
		}
	}

	if ssmClient == nil || invocations == nil {
		// Use a retryable error message - SSM commands may take time to appear in API
		return nil, fmt.Errorf("SSM command %s timeout - command may still be initializing in AWS API", commandID)
	}

	// Build result
	cmdResult := &provider.CommandResult{
		CommandID: commandID,
		Status:    "Success",
		Instances: make(map[string]*provider.InstanceCommandResult),
	}

	allComplete := true
	for _, inv := range invocations.CommandInvocations {
		instanceID := aws.ToString(inv.InstanceId)
		status := string(inv.Status)

		if status == "InProgress" || status == "Pending" {
			allComplete = false
			cmdResult.Status = "InProgress"
			cmdResult.Instances[instanceID] = &provider.InstanceCommandResult{
				InstanceID: instanceID,
				Status:     status,
			}
			continue
		}

		// Get command output for completed commands
		output, err := ssmClient.GetCommandInvocation(ctx, &ssm.GetCommandInvocationInput{
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

	// If any commands are still in progress, return InProgress status
	if !allComplete {
		cmdResult.Status = "InProgress"
	}

	return cmdResult, nil
}
