package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/sarth-shah20/stasis/internal/config"
)

// AWSProvider implements Provider by provisioning real AWS managed services.
// It supports three cloud.type values: "postgres" (RDS), "redis" (ElastiCache), "storage" (S3).
//
// AWS credentials and region come from the standard environment variables
// (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_REGION) via LoadDefaultConfig,
// following the same pattern as internal/remote/s3.go.
type AWSProvider struct {
	rdsClient         *rds.Client
	elasticacheClient *elasticache.Client
	s3Client          *s3.Client
}

// NewAWSProvider initializes AWS SDK clients for RDS, ElastiCache, and S3.
func NewAWSProvider(ctx context.Context) (*AWSProvider, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	return &AWSProvider{
		rdsClient:         rds.NewFromConfig(cfg),
		elasticacheClient: elasticache.NewFromConfig(cfg),
		s3Client:          s3.NewFromConfig(cfg),
	}, nil
}

// resourceName returns the consistent stasis-{project}-{service} identifier.
func resourceName(projectName, serviceName string) string {
	return fmt.Sprintf("stasis-%s-%s", projectName, serviceName)
}

// getEnvValue extracts a value from KEY=VALUE style environment entries.
func getEnvValue(envs []string, key string) string {
	prefix := key + "="
	for _, e := range envs {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix)
		}
	}
	return ""
}

// Provision creates the appropriate AWS resource based on cloud.type.
// It returns immediately (fire-and-forget); use Status() to check readiness.
func (a *AWSProvider) Provision(ctx context.Context, projectName string, serviceName string, service config.Service) (ConnectionInfo, error) {
	switch service.Cloud.Type {
	case "postgres":
		return a.provisionRDS(ctx, projectName, serviceName, service)
	case "redis":
		return a.provisionElastiCache(ctx, projectName, serviceName, service)
	case "storage":
		return a.provisionS3(ctx, projectName, serviceName)
	default:
		return ConnectionInfo{}, fmt.Errorf("unsupported cloud type: %q (supported: postgres, redis, storage)", service.Cloud.Type)
	}
}

// Deprovision removes the AWS resource for the given service.
func (a *AWSProvider) Deprovision(ctx context.Context, projectName string, serviceName string) error {
	// We don't know the cloud.type from just project+service name, so we try all three.
	// Only one will match; the others return "not found" errors which we ignore.
	name := resourceName(projectName, serviceName)
	var lastErr error

	// Try RDS
	if err := a.deprovisionRDS(ctx, name); err != nil {
		if !isAWSNotFound(err) {
			lastErr = err
		}
	} else {
		fmt.Printf("  🗑  Deleting RDS instance: %s\n", name)
		return nil
	}

	// Try ElastiCache
	if err := a.deprovisionElastiCache(ctx, name); err != nil {
		if !isAWSNotFound(err) {
			lastErr = err
		}
	} else {
		fmt.Printf("  🗑  Deleting ElastiCache cluster: %s\n", name)
		return nil
	}

	// Try S3
	if err := a.deprovisionS3(ctx, name); err != nil {
		if !isAWSNotFound(err) {
			lastErr = err
		}
	} else {
		fmt.Printf("  🗑  Deleting S3 bucket: %s\n", name)
		return nil
	}

	if lastErr != nil {
		return fmt.Errorf("failed to deprovision %s: %w", name, lastErr)
	}
	return fmt.Errorf("no AWS resource found for %s", name)
}

// Status returns the current state of the AWS resource.
func (a *AWSProvider) Status(ctx context.Context, projectName string, serviceName string) (string, error) {
	name := resourceName(projectName, serviceName)

	// Try RDS
	if status, err := a.statusRDS(ctx, name); err == nil {
		return status, nil
	}

	// Try ElastiCache
	if status, err := a.statusElastiCache(ctx, name); err == nil {
		return status, nil
	}

	// Try S3
	if status, err := a.statusS3(ctx, name); err == nil {
		return status, nil
	}

	return "not found", nil
}

// ---------------------------------------------------------------------------
// RDS (Postgres)
// ---------------------------------------------------------------------------

func (a *AWSProvider) provisionRDS(ctx context.Context, projectName, serviceName string, service config.Service) (ConnectionInfo, error) {
	name := resourceName(projectName, serviceName)

	// Determine instance class from tier
	instanceClass := "db.t3.micro" // free
	allocatedStorage := int32(20)
	if service.Cloud.Tier == "standard" {
		instanceClass = "db.t3.small"
		allocatedStorage = 50
	}

	// Extract credentials from environment vars defined in stasis.yaml
	masterPassword := getEnvValue(service.Environment, "POSTGRES_PASSWORD")
	if masterPassword == "" {
		masterPassword = "stasis-auto-pwd"
	}
	dbName := getEnvValue(service.Environment, "POSTGRES_DB")
	if dbName == "" {
		dbName = "stasis"
	}
	masterUser := getEnvValue(service.Environment, "POSTGRES_USER")
	if masterUser == "" {
		masterUser = "postgres"
	}

	fmt.Printf("  ☁️  Creating RDS PostgreSQL instance: %s (%s)...\n", name, instanceClass)

	input := &rds.CreateDBInstanceInput{
		DBInstanceIdentifier: aws.String(name),
		DBInstanceClass:      aws.String(instanceClass),
		Engine:               aws.String("postgres"),
		EngineVersion:        aws.String("14"),
		MasterUsername:       aws.String(masterUser),
		MasterUserPassword:   aws.String(masterPassword),
		DBName:               aws.String(dbName),
		AllocatedStorage:     aws.Int32(allocatedStorage),
		StorageType:          aws.String("gp2"),
		PubliclyAccessible:   aws.Bool(true),
		EnableCloudwatchLogsExports: []string{"postgresql"},
		Tags: []rdstypes.Tag{
			{Key: aws.String("stasis.project"), Value: aws.String(projectName)},
			{Key: aws.String("stasis.service"), Value: aws.String(serviceName)},
			{Key: aws.String("stasis.managed"), Value: aws.String("true")},
		},
	}

	result, err := a.rdsClient.CreateDBInstance(ctx, input)
	if err != nil {
		return ConnectionInfo{}, fmt.Errorf("failed to create RDS instance %s: %w", name, err)
	}

	info := ConnectionInfo{Port: 5432}
	if result.DBInstance != nil && result.DBInstance.Endpoint != nil {
		info.Host = aws.ToString(result.DBInstance.Endpoint.Address)
		if result.DBInstance.Endpoint.Port != nil {
			info.Port = int(*result.DBInstance.Endpoint.Port)
		}
		info.Endpoint = fmt.Sprintf("%s:%d", info.Host, info.Port)
	} else {
		// Instance is still creating; endpoint not yet available
		info.Endpoint = "(creating... check with stasis status --cloud)"
	}

	return info, nil
}

func (a *AWSProvider) deprovisionRDS(ctx context.Context, name string) error {
	_, err := a.rdsClient.DeleteDBInstance(ctx, &rds.DeleteDBInstanceInput{
		DBInstanceIdentifier: aws.String(name),
		SkipFinalSnapshot:    aws.Bool(true), // Dev environments — no final snapshot
	})
	return err
}

func (a *AWSProvider) statusRDS(ctx context.Context, name string) (string, error) {
	result, err := a.rdsClient.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(name),
	})
	if err != nil {
		return "", err
	}

	if len(result.DBInstances) == 0 {
		return "", fmt.Errorf("not found")
	}

	instance := result.DBInstances[0]
	status := aws.ToString(instance.DBInstanceStatus)

	endpoint := "pending"
	if instance.Endpoint != nil {
		port := int32(5432)
		if instance.Endpoint.Port != nil {
			port = *instance.Endpoint.Port
		}
		endpoint = fmt.Sprintf("%s:%d", aws.ToString(instance.Endpoint.Address), port)
	}

	return fmt.Sprintf("RDS postgres | %s | %s", status, endpoint), nil
}

// ---------------------------------------------------------------------------
// ElastiCache (Redis)
// ---------------------------------------------------------------------------

func (a *AWSProvider) provisionElastiCache(ctx context.Context, projectName, serviceName string, service config.Service) (ConnectionInfo, error) {
	name := resourceName(projectName, serviceName)

	nodeType := "cache.t3.micro" // free
	if service.Cloud.Tier == "standard" {
		nodeType = "cache.t3.small"
	}

	fmt.Printf("  ☁️  Creating ElastiCache Redis cluster: %s (%s)...\n", name, nodeType)

	input := &elasticache.CreateCacheClusterInput{
		CacheClusterId: aws.String(name),
		CacheNodeType:  aws.String(nodeType),
		CacheSubnetGroupName: aws.String("stasis-default"),
		Engine:         aws.String("redis"),
		NumCacheNodes:  aws.Int32(1),
		Tags: []elasticachetypes.Tag{
			{Key: aws.String("stasis.project"), Value: aws.String(projectName)},
			{Key: aws.String("stasis.service"), Value: aws.String(serviceName)},
			{Key: aws.String("stasis.managed"), Value: aws.String("true")},
		},
	}

	_, err := a.elasticacheClient.CreateCacheCluster(ctx, input)
	if err != nil {
		return ConnectionInfo{}, fmt.Errorf("failed to create ElastiCache cluster %s: %w", name, err)
	}

	// ElastiCache endpoints aren't available immediately
	info := ConnectionInfo{
		Port:     6379,
		Endpoint: "(creating... check with stasis status --cloud)",
	}

	return info, nil
}

func (a *AWSProvider) deprovisionElastiCache(ctx context.Context, name string) error {
	_, err := a.elasticacheClient.DeleteCacheCluster(ctx, &elasticache.DeleteCacheClusterInput{
		CacheClusterId: aws.String(name),
	})
	return err
}

func (a *AWSProvider) statusElastiCache(ctx context.Context, name string) (string, error) {
	result, err := a.elasticacheClient.DescribeCacheClusters(ctx, &elasticache.DescribeCacheClustersInput{
		CacheClusterId:    aws.String(name),
		ShowCacheNodeInfo: aws.Bool(true),
	})
	if err != nil {
		return "", err
	}

	if len(result.CacheClusters) == 0 {
		return "", fmt.Errorf("not found")
	}

	cluster := result.CacheClusters[0]
	status := aws.ToString(cluster.CacheClusterStatus)

	endpoint := "pending"
	if len(cluster.CacheNodes) > 0 && cluster.CacheNodes[0].Endpoint != nil {
		ep := cluster.CacheNodes[0].Endpoint
		port := int32(6379)
		if ep.Port != nil {
			port = *ep.Port
		}
		endpoint = fmt.Sprintf("%s:%d", aws.ToString(ep.Address), port)
	}

	return fmt.Sprintf("ElastiCache redis | %s | %s", status, endpoint), nil
}

// ---------------------------------------------------------------------------
// S3 (Storage)
// ---------------------------------------------------------------------------

func (a *AWSProvider) provisionS3(ctx context.Context, projectName, serviceName string) (ConnectionInfo, error) {
	name := resourceName(projectName, serviceName)

	fmt.Printf("  ☁️  Creating S3 bucket: %s...\n", name)

	_, err := a.s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(name),
	})
	if err != nil {
		return ConnectionInfo{}, fmt.Errorf("failed to create S3 bucket %s: %w", name, err)
	}

	info := ConnectionInfo{
		Endpoint: fmt.Sprintf("s3://%s", name),
	}

	return info, nil
}

func (a *AWSProvider) deprovisionS3(ctx context.Context, name string) error {
	// S3 buckets must be empty before deletion.
	// For dev environments, we empty and then delete.

	// 1. List and delete all objects
	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(name),
	}

	listResult, err := a.s3Client.ListObjectsV2(ctx, listInput)
	if err != nil {
		return err
	}

	for _, obj := range listResult.Contents {
		_, err := a.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(name),
			Key:    obj.Key,
		})
		if err != nil {
			return fmt.Errorf("failed to delete object %s: %w", aws.ToString(obj.Key), err)
		}
	}

	// 2. Delete the bucket itself
	_, err = a.s3Client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(name),
	})
	return err
}

func (a *AWSProvider) statusS3(ctx context.Context, name string) (string, error) {
	_, err := a.s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(name),
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("S3 bucket | active | s3://%s", name), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isAWSNotFound returns true if the error indicates the resource doesn't exist.
// Works across RDS, ElastiCache, and S3 "not found" style errors.
func isAWSNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "NotFound") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "NoSuchBucket") ||
		strings.Contains(msg, "NoSuchEntity") ||
		strings.Contains(msg, "DBInstanceNotFound") ||
		strings.Contains(msg, "CacheClusterNotFound")
}
