package remote

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Client struct {
	client     *s3.Client
	uploader   *manager.Uploader
	downloader *manager.Downloader
	bucket     string
}

// NewS3Client initializes a connection to AWS using local credentials
func NewS3Client(ctx context.Context, region, bucket string) (*S3Client, error) {
	if bucket == "" {
		return nil, fmt.Errorf("S3 bucket name is required in stasis.yaml")
	}

	// LoadDefaultConfig automatically looks for ~/.aws/credentials or Env Vars
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	client := s3.NewFromConfig(cfg)

	return &S3Client{
		client:     client,
		uploader:   manager.NewUploader(client),
		downloader: manager.NewDownloader(client),
		bucket:     bucket,
	}, nil
}

// Push uploads an entire local snapshot directory to S3
func (s *S3Client) Push(ctx context.Context, projectName, snapshotName, localDir string) error {
	// Read all files in the local snapshot directory
	entries, err := os.ReadDir(localDir)
	if err != nil {
		return fmt.Errorf("failed to read local snapshot dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip subdirectories for now to keep it simple
		}

		localFilePath := filepath.Join(localDir, entry.Name())
		
		// S3 Key: stasis-snapshots/<project>/<snapshot>/<filename>
		s3Key := fmt.Sprintf("stasis-snapshots/%s/%s/%s", projectName, snapshotName, entry.Name())

		// Open the file for reading
		file, err := os.Open(localFilePath)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", localFilePath, err)
		}

		fmt.Printf("  -> Uploading %s to S3...\n", entry.Name())

		// Upload to S3
		_, err = s.uploader.Upload(ctx, &s3.PutObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(s3Key),
			Body:   file, // We pass the file handle directly!
		})
		
		file.Close() // Close immediately after upload

		if err != nil {
			return fmt.Errorf("failed to upload %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// Pull downloads a snapshot from S3 to the local directory
func (s *S3Client) Pull(ctx context.Context, projectName, snapshotName, localDir string) error {
	// The "folder" path in S3
	prefix := fmt.Sprintf("stasis-snapshots/%s/%s/", projectName, snapshotName)

	// 1. List all files in the S3 directory
	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	}

	listOutput, err := s.client.ListObjectsV2(ctx, listInput)
	if err != nil {
		return fmt.Errorf("failed to list objects in S3: %w", err)
	}

	if len(listOutput.Contents) == 0 {
		return fmt.Errorf("no snapshot found in S3 for %s/%s", projectName, snapshotName)
	}

	// 2. Ensure local directory exists
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("failed to create local snapshot dir: %w", err)
	}

	// 3. Download each file
	for _, object := range listOutput.Contents {
		// Extract just the filename from the S3 key 
		// (e.g., "stasis-snapshots/proj/snap/dump.sql" -> "dump.sql")
		fileName := filepath.Base(*object.Key)
		localFilePath := filepath.Join(localDir, fileName)

		fmt.Printf("  <- Downloading %s from S3...\n", fileName)

		// Create the local file
		file, err := os.Create(localFilePath)
		if err != nil {
			return fmt.Errorf("failed to create local file %s: %w", localFilePath, err)
		}

		// Download the data into the file
		// The downloader automatically handles concurrent chunk downloads!
		_, err = s.downloader.Download(ctx, file, &s3.GetObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    object.Key,
		})
		
		// We close the file explicitly inside the loop instead of using defer.
		// Using defer inside a loop is a classic Go bug (it waits until the function exits to close them all).
		file.Close() 

		if err != nil {
			return fmt.Errorf("failed to download %s: %w", fileName, err)
		}
	}

	return nil
}