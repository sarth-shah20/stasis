package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/spf13/cobra"
)

var (
	cloudLogs bool
	logLines  int
)

var logsCmd = &cobra.Command{
	Use:   "logs [service]",
	Short: "Fetch logs for a service",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		serviceName := args[0]

		if !cloudLogs {
			fmt.Printf("Use stasis logs %s --cloud for cloud logs. Local logs: docker logs stasis-%s-%s\n",
				serviceName, cfg.Name, serviceName)
			return nil
		}

		return runCloudLogs(context.Background(), serviceName)
	},
}

func runCloudLogs(ctx context.Context, serviceName string) error {
	service, ok := cfg.Services[serviceName]
	if !ok {
		return fmt.Errorf("service %q not found in stasis.yaml", serviceName)
	}

	if service.Cloud.Type == "" {
		return fmt.Errorf("service %q has no cloud configuration", serviceName)
	}

	switch service.Cloud.Type {
	case "postgres":
		return fetchRDSLogs(ctx, serviceName)
	case "redis":
		fmt.Println("ElastiCache slow logs require Redis 6.2+. Run stasis status --cloud to check cluster health instead.")
		return nil
	case "storage":
		fmt.Println("S3 access logs are not enabled by default. Enable via AWS Console > S3 > Bucket > Properties > Server access logging.")
		return nil
	default:
		return fmt.Errorf("unsupported cloud type %q for logs", service.Cloud.Type)
	}
}

func fetchRDSLogs(ctx context.Context, serviceName string) error {
	sdkCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("unable to load AWS SDK config: %w", err)
	}
	client := cloudwatchlogs.NewFromConfig(sdkCfg)

	logGroup := fmt.Sprintf("/aws/rds/instance/stasis-%s-%s/postgresql", cfg.Name, serviceName)

	fmt.Printf("\n📋 Cloud Logs — %s (postgres)\n", serviceName)
	fmt.Printf("   Log Group: %s\n", logGroup)
	fmt.Printf("   Last %d lines\n\n", logLines)

	// Fetch log events from the last 24 hours
	startTime := time.Now().Add(-24 * time.Hour).UnixMilli()
	endTime := time.Now().UnixMilli()

	input := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String(logGroup),
		StartTime:    aws.Int64(startTime),
		EndTime:      aws.Int64(endTime),
		Limit:        aws.Int32(int32(logLines)),
	}

	result, err := client.FilterLogEvents(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to fetch CloudWatch logs: %w", err)
	}

	if len(result.Events) == 0 {
		fmt.Println("   No log events found in the last 24 hours.")
		return nil
	}

	// Print table header
	separator := strings.Repeat("─", 90)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "TIMESTAMP\tLEVEL\tMESSAGE\n")
	fmt.Fprintf(w, "%s\n", separator)

	for _, event := range result.Events {
		ts := time.UnixMilli(aws.ToInt64(event.Timestamp)).Format("2006-01-02 15:04:05")
		msg := aws.ToString(event.Message)

		// Attempt to parse log level from the message
		level := parseLogLevel(msg)

		// Trim the message for display (remove newlines, truncate)
		msg = strings.TrimSpace(msg)
		if len(msg) > 120 {
			msg = msg[:117] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n", ts, level, msg)
	}

	w.Flush()
	fmt.Println()
	return nil
}

// parseLogLevel attempts to extract a PostgreSQL log level from the message.
func parseLogLevel(msg string) string {
	levels := []string{"FATAL", "ERROR", "WARNING", "LOG", "INFO", "DEBUG", "NOTICE", "STATEMENT"}
	upper := strings.ToUpper(msg)
	for _, l := range levels {
		if strings.Contains(upper, l+":") || strings.HasPrefix(upper, l) {
			return l
		}
	}
	return "LOG"
}

func init() {
	logsCmd.Flags().BoolVar(&cloudLogs, "cloud", false, "Fetch logs from cloud (AWS CloudWatch)")
	logsCmd.Flags().IntVar(&logLines, "lines", 50, "Number of log lines to fetch")
	rootCmd.AddCommand(logsCmd)
}
