package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
	"github.com/madhouselabs/goman/pkg/provider/aws"
	"github.com/madhouselabs/goman/pkg/provider/registry"
	"github.com/madhouselabs/goman/pkg/setup"
)

// CLI handles command-line interface operations
type CLI struct {
	initCmd    *flag.FlagSet
	statusCmd  *flag.FlagSet
	clusterCmd *flag.FlagSet
}

// NewCLI creates a new CLI handler
func NewCLI() *CLI {
	cli := &CLI{
		initCmd:    flag.NewFlagSet("init", flag.ExitOnError),
		statusCmd:  flag.NewFlagSet("status", flag.ExitOnError),
		clusterCmd: flag.NewFlagSet("cluster", flag.ExitOnError),
	}

	// Add flags for init command
	cli.initCmd.Bool("force", false, "Force re-initialization even if already initialized")

	return cli
}

// Run processes CLI arguments and executes appropriate command
func (cli *CLI) Run() {
	// If no arguments, run TUI
	if len(os.Args) < 2 {
		cli.runTUI()
		return
	}

	switch os.Args[1] {
	case "init", "--init":
		cli.handleInit()
	case "status", "--status":
		cli.handleStatus()
	case "cluster":
		cli.handleCluster()
	case "uninit":
		// Hidden command - not shown in help
		cli.handleUninit()
	case "help", "--help", "-h":
		cli.printHelp()
	default:
		// Unknown command, run TUI
		cli.runTUI()
	}
}

// handleInit handles the initialization command
func (cli *CLI) handleInit() {
	// When called directly via 'goman init', show the TUI initialization screen
	// This provides a better user experience than CLI-only mode
	cli.showInitPrompt()
}

// handleUninit removes all Goman infrastructure (hidden command)
func (cli *CLI) handleUninit() {
	fmt.Println("WARNING: This will remove all Goman infrastructure from AWS")
	fmt.Println("This includes:")
	fmt.Println("  • S3 bucket and all stored data")
	fmt.Println("  • Lambda function")
	fmt.Println("  • DynamoDB table")
	fmt.Println("  • IAM roles and policies")
	fmt.Println()
	fmt.Print("Are you sure? (yes/no): ")
	
	var response string
	fmt.Scanln(&response)
	
	if response != "yes" {
		fmt.Println("Aborted.")
		return
	}
	
	fmt.Println("\nRemoving Goman infrastructure...")
	
	// Run cleanup
	ctx := context.Background()
	if err := cli.cleanupInfrastructure(ctx); err != nil {
		fmt.Printf("Error during cleanup: %v\n", err)
		fmt.Println("Some resources may need manual cleanup in AWS console")
		os.Exit(1)
	}
	
	// Remove initialization marker
	home, _ := os.UserHomeDir()
	initFile := filepath.Join(home, ".goman", "initialized.json")
	os.Remove(initFile)
	
	fmt.Println("✓ Goman infrastructure removed successfully")
	fmt.Println("Run 'goman init' to reinitialize when needed")
}

// handleStatus shows initialization status
func (cli *CLI) handleStatus() {
	if !cli.isInitialized() {
		fmt.Println("✗ Goman is not initialized")
		fmt.Println("Run 'goman init' to set up the infrastructure")
		os.Exit(1)
	}

	status := cli.getInitStatus()
	fmt.Println("Goman Infrastructure Status:")
	fmt.Println("============================")
	if status.S3Bucket {
		fmt.Println("✓ S3 Bucket: Configured")
	} else {
		fmt.Println("✗ S3 Bucket: Not configured")
	}
	if status.Lambda {
		fmt.Println("✓ Lambda Function: Deployed")
	} else {
		fmt.Println("✗ Lambda Function: Not deployed")
	}
	if status.DynamoDB {
		fmt.Println("✓ DynamoDB Table: Created")
	} else {
		fmt.Println("✗ DynamoDB Table: Not created")
	}
	if status.IAMRoles {
		fmt.Println("✓ IAM Roles: Configured")
	} else {
		fmt.Println("✗ IAM Roles: Not configured")
	}
}

// handleCluster handles cluster subcommands
func (cli *CLI) handleCluster() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: goman cluster <command>")
		fmt.Println("Commands: create, delete, list")
		os.Exit(1)
	}

	// Check initialization first
	if !cli.isInitialized() {
		fmt.Println("✗ Goman is not initialized")
		fmt.Println("Run 'goman init' first to set up the infrastructure")
		os.Exit(1)
	}

	// Handle cluster subcommands
	switch os.Args[2] {
	case "create":
		fmt.Println("Cluster creation should be done through the TUI")
		fmt.Println("Run 'goman' to access the interactive interface")
	case "delete":
		fmt.Println("Cluster deletion should be done through the TUI")
		fmt.Println("Run 'goman' to access the interactive interface")
	case "list":
		fmt.Println("Cluster listing should be done through the TUI")
		fmt.Println("Run 'goman' to access the interactive interface")
	default:
		fmt.Printf("Unknown cluster command: %s\n", os.Args[2])
		os.Exit(1)
	}
}

// runTUI runs the interactive TUI
func (cli *CLI) runTUI() {
	// Check initialization status
	if !cli.isInitialized() {
		// Show initialization prompt
		cli.showInitPrompt()
		return
	}

	// Run the normal TUI
	runMainTUI()
}

// showInitPrompt shows initialization prompt in TUI
func (cli *CLI) showInitPrompt() {
	// Initialize bubblezone manager for mouse support
	zone.NewGlobal()
	
	p := tea.NewProgram(newInitPromptModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	
	// After initialization completes, check if we should run the main TUI
	if cli.isInitialized() {
		runMainTUI()
	}
}

// isInitialized checks if Goman is initialized
func (cli *CLI) isInitialized() bool {
	// Check for initialization marker file
	home, _ := os.UserHomeDir()
	initFile := filepath.Join(home, ".goman", "initialized.json")
	
	if _, err := os.Stat(initFile); os.IsNotExist(err) {
		return false
	}

	// Verify the status
	status := cli.getInitStatus()
	return status.S3Bucket && status.Lambda && status.DynamoDB && status.IAMRoles
}

// InitStatus represents initialization status
type InitStatus struct {
	S3Bucket   bool `json:"s3_bucket"`
	Lambda     bool `json:"lambda"`
	DynamoDB   bool `json:"dynamodb"`
	IAMRoles   bool `json:"iam_roles"`
	Timestamp  string `json:"timestamp"`
}

// getInitStatus reads initialization status
func (cli *CLI) getInitStatus() InitStatus {
	home, _ := os.UserHomeDir()
	initFile := filepath.Join(home, ".goman", "initialized.json")
	
	data, err := os.ReadFile(initFile)
	if err != nil {
		return InitStatus{}
	}

	var status InitStatus
	json.Unmarshal(data, &status)
	return status
}

// saveInitStatus saves initialization status
func saveInitStatus(result *setup.InitializeResult) error {
	home, _ := os.UserHomeDir()
	gomanDir := filepath.Join(home, ".goman")
	
	// Create directory if it doesn't exist
	if err := os.MkdirAll(gomanDir, 0755); err != nil {
		return err
	}

	status := InitStatus{
		S3Bucket:  result.S3BucketCreated,
		Lambda:    result.LambdaDeployed,
		DynamoDB:  result.DynamoDBCreated,
		IAMRoles:  result.SSMProfileCreated,
		Timestamp: fmt.Sprintf("%v", result),
	}

	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}

	initFile := filepath.Join(gomanDir, "initialized.json")
	return os.WriteFile(initFile, data, 0644)
}

// cleanupInfrastructure removes all AWS resources
func (cli *CLI) cleanupInfrastructure(ctx context.Context) error {
	// Get AWS provider to access resource names
	provider, err := registry.GetDefaultProvider()
	if err != nil {
		return fmt.Errorf("failed to get provider: %w", err)
	}

	awsProvider, ok := provider.(*aws.AWSProvider)
	if !ok {
		return fmt.Errorf("provider is not AWS")
	}

	accountID := awsProvider.AccountID()
	region := awsProvider.Region()
	
	// Resource names
	bucketName := fmt.Sprintf("goman-%s", accountID)
	functionName := "goman-cluster-controller"
	tableName := "goman-resource-locks"
	lambdaRoleName := fmt.Sprintf("goman-lambda-role-%s", accountID)
	lambdaPolicyName := fmt.Sprintf("goman-lambda-policy-%s", accountID)
	ssmRoleName := "goman-ssm-instance-role"
	ssmProfileName := "goman-ssm-instance-profile"
	
	// Create AWS clients
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return err
	}
	
	s3Client := s3.NewFromConfig(cfg)
	lambdaClient := lambda.NewFromConfig(cfg)
	dynamoClient := dynamodb.NewFromConfig(cfg)
	iamClient := iam.NewFromConfig(cfg)
	
	var errors []string
	
	// 1. Delete EventBridge rule and Lambda function
	fmt.Println("  • Deleting EventBridge rules...")
	eventClient := eventbridge.NewFromConfig(cfg)
	ruleName := "goman-ec2-state-change-rule"
	
	// Remove targets first
	eventClient.RemoveTargets(ctx, &eventbridge.RemoveTargetsInput{
		Rule: awssdk.String(ruleName),
		Ids:  []string{"1"},
	})
	
	// Delete the rule
	eventClient.DeleteRule(ctx, &eventbridge.DeleteRuleInput{
		Name: awssdk.String(ruleName),
	})
	
	fmt.Println("  • Deleting Lambda function...")
	_, err = lambdaClient.DeleteFunction(ctx, &lambda.DeleteFunctionInput{
		FunctionName: awssdk.String(functionName),
	})
	if err != nil && !strings.Contains(err.Error(), "ResourceNotFoundException") {
		errors = append(errors, fmt.Sprintf("Lambda: %v", err))
	}
	
	// 2. Delete S3 bucket (must be empty first)
	fmt.Println("  • Deleting S3 bucket...")
	// First, delete all objects
	listOutput, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: awssdk.String(bucketName),
	})
	if err == nil && listOutput.Contents != nil {
		for _, obj := range listOutput.Contents {
			s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: awssdk.String(bucketName),
				Key:    obj.Key,
			})
		}
	}
	// Then delete bucket
	_, err = s3Client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: awssdk.String(bucketName),
	})
	if err != nil && !strings.Contains(err.Error(), "NoSuchBucket") {
		errors = append(errors, fmt.Sprintf("S3: %v", err))
	}
	
	// 3. Delete DynamoDB table
	fmt.Println("  • Deleting DynamoDB table...")
	_, err = dynamoClient.DeleteTable(ctx, &dynamodb.DeleteTableInput{
		TableName: awssdk.String(tableName),
	})
	if err != nil && !strings.Contains(err.Error(), "ResourceNotFoundException") {
		errors = append(errors, fmt.Sprintf("DynamoDB: %v", err))
	}
	
	// 4. Delete IAM resources
	fmt.Println("  • Deleting IAM roles and policies...")
	
	// Remove role from instance profile
	iamClient.RemoveRoleFromInstanceProfile(ctx, &iam.RemoveRoleFromInstanceProfileInput{
		InstanceProfileName: awssdk.String(ssmProfileName),
		RoleName:           awssdk.String(ssmRoleName),
	})
	
	// Delete instance profile
	iamClient.DeleteInstanceProfile(ctx, &iam.DeleteInstanceProfileInput{
		InstanceProfileName: awssdk.String(ssmProfileName),
	})
	
	// Detach and delete policies
	iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
		RoleName:  awssdk.String(ssmRoleName),
		PolicyArn: awssdk.String("arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"),
	})
	iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: awssdk.String(ssmRoleName),
	})
	
	// Detach ALL policies from Lambda role before deletion
	listAttached, err := iamClient.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{
		RoleName: awssdk.String(lambdaRoleName),
	})
	if err == nil && listAttached.AttachedPolicies != nil {
		for _, policy := range listAttached.AttachedPolicies {
			iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
				RoleName:  awssdk.String(lambdaRoleName),
				PolicyArn: policy.PolicyArn,
			})
		}
	}
	
	// Delete custom policy (first delete all non-default versions)
	policyArn := fmt.Sprintf("arn:aws:iam::%s:policy/%s", accountID, lambdaPolicyName)
	listVersionsOutput, err := iamClient.ListPolicyVersions(ctx, &iam.ListPolicyVersionsInput{
		PolicyArn: awssdk.String(policyArn),
	})
	if err == nil && listVersionsOutput.Versions != nil {
		for _, version := range listVersionsOutput.Versions {
			if !version.IsDefaultVersion {
				iamClient.DeletePolicyVersion(ctx, &iam.DeletePolicyVersionInput{
					PolicyArn: awssdk.String(policyArn),
					VersionId: version.VersionId,
				})
			}
		}
	}
	iamClient.DeletePolicy(ctx, &iam.DeletePolicyInput{
		PolicyArn: awssdk.String(policyArn),
	})
	
	// Delete Lambda role
	iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: awssdk.String(lambdaRoleName),
	})
	
	if len(errors) > 0 {
		return fmt.Errorf("some resources failed to delete: %s", strings.Join(errors, "; "))
	}
	
	return nil
}

// printHelp prints help information
func (cli *CLI) printHelp() {
	fmt.Println("Goman - Kubernetes Cluster Manager")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  goman              Start interactive TUI")
	fmt.Println("  goman init         Initialize infrastructure")
	fmt.Println("  goman status       Show initialization status")
	fmt.Println("  goman cluster      Manage clusters (use TUI)")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --help, -h         Show this help message")
	fmt.Println()
	fmt.Println("First-time setup:")
	fmt.Println("  Run 'goman init' to set up AWS infrastructure before using the TUI")
}

