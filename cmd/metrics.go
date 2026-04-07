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
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/spf13/cobra"
)

var cloudMetrics bool

var metricsCmd = &cobra.Command{
	Use:   "metrics [service]",
	Short: "Show CloudWatch metrics for a cloud service",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		serviceName := args[0]

		if !cloudMetrics {
			fmt.Printf("Use stasis metrics %s --cloud for cloud metrics. Local metrics are available via Docker stats.\n",
				serviceName)
			return nil
		}

		return runCloudMetrics(context.Background(), serviceName)
	},
}

// metricDef describes a single metric to fetch and how to display it.
type metricDef struct {
	Name      string  // CloudWatch metric name
	Unit      string  // Display unit
	Converter func(float64) float64 // Optional value conversion
}

func runCloudMetrics(ctx context.Context, serviceName string) error {
	service, ok := cfg.Services[serviceName]
	if !ok {
		return fmt.Errorf("service %q not found in stasis.yaml", serviceName)
	}

	if service.Cloud.Type == "" {
		return fmt.Errorf("service %q has no cloud configuration", serviceName)
	}

	switch service.Cloud.Type {
	case "postgres":
		return fetchMetrics(ctx, serviceName, "AWS/RDS", "DBInstanceIdentifier", rdsMetrics())
	case "redis":
		return fetchMetrics(ctx, serviceName, "AWS/ElastiCache", "CacheClusterId", elasticacheMetrics())
	case "storage":
		name := fmt.Sprintf("stasis-%s-%s", cfg.Name, serviceName)
		fmt.Printf("S3 metrics require enabling request metrics in AWS Console. Bucket: %s\n", name)
		return nil
	default:
		return fmt.Errorf("unsupported cloud type %q for metrics", service.Cloud.Type)
	}
}

func rdsMetrics() []metricDef {
	return []metricDef{
		{Name: "CPUUtilization", Unit: "%", Converter: nil},
		{Name: "DatabaseConnections", Unit: "count", Converter: nil},
		{Name: "FreeStorageSpace", Unit: "GB", Converter: bytesToGB},
		{Name: "ReadLatency", Unit: "ms", Converter: secondsToMS},
		{Name: "WriteLatency", Unit: "ms", Converter: secondsToMS},
	}
}

func elasticacheMetrics() []metricDef {
	return []metricDef{
		{Name: "CPUUtilization", Unit: "%", Converter: nil},
		{Name: "CacheHits", Unit: "count", Converter: nil},
		{Name: "CacheMisses", Unit: "count", Converter: nil},
		{Name: "CurrConnections", Unit: "count", Converter: nil},
		{Name: "NetworkBytesIn", Unit: "KB", Converter: bytesToKB},
		{Name: "NetworkBytesOut", Unit: "KB", Converter: bytesToKB},
	}
}

func bytesToGB(v float64) float64  { return v / (1024 * 1024 * 1024) }
func bytesToKB(v float64) float64  { return v / 1024 }
func secondsToMS(v float64) float64 { return v * 1000 }

func fetchMetrics(ctx context.Context, serviceName, namespace, dimensionName string, metrics []metricDef) error {
	sdkCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("unable to load AWS SDK config: %w", err)
	}
	client := cloudwatch.NewFromConfig(sdkCfg)

	resourceName := fmt.Sprintf("stasis-%s-%s", cfg.Name, serviceName)
	endTime := time.Now()
	startTime := endTime.Add(-1 * time.Hour)
	period := int32(300) // 5-minute periods

	fmt.Printf("\n📊 Cloud Metrics — %s\n", serviceName)
	fmt.Printf("   Resource: %s | Namespace: %s\n", resourceName, namespace)
	fmt.Printf("   Period: last 1 hour (5-min avg)\n\n")

	separator := strings.Repeat("─", 70)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', tabwriter.AlignRight)
	fmt.Fprintf(w, "METRIC\tCURRENT\tAVG (1H)\tUNIT\t\n")
	fmt.Fprintf(w, "%s\t\t\t\t\n", separator)

	for _, m := range metrics {
		input := &cloudwatch.GetMetricStatisticsInput{
			Namespace:  aws.String(namespace),
			MetricName: aws.String(m.Name),
			Dimensions: []cwtypes.Dimension{
				{
					Name:  aws.String(dimensionName),
					Value: aws.String(resourceName),
				},
			},
			StartTime:  aws.Time(startTime),
			EndTime:    aws.Time(endTime),
			Period:     aws.Int32(period),
			Statistics: []cwtypes.Statistic{cwtypes.StatisticAverage},
		}

		result, err := client.GetMetricStatistics(ctx, input)
		if err != nil {
			fmt.Fprintf(w, "%s\terror\terror\t%s\t\n", m.Name, m.Unit)
			continue
		}

		current, avg := computeMetricValues(result.Datapoints, m.Converter)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t\n", m.Name, current, avg, m.Unit)
	}

	w.Flush()
	fmt.Println()
	return nil
}

// computeMetricValues extracts the latest and average values from datapoints.
func computeMetricValues(datapoints []cwtypes.Datapoint, converter func(float64) float64) (string, string) {
	if len(datapoints) == 0 {
		return "—", "—"
	}

	// Find latest datapoint (by timestamp)
	var latest cwtypes.Datapoint
	for i, dp := range datapoints {
		if i == 0 || dp.Timestamp.After(*latest.Timestamp) {
			latest = dp
		}
	}

	// Compute average across all datapoints
	var sum float64
	for _, dp := range datapoints {
		if dp.Average != nil {
			sum += *dp.Average
		}
	}
	avg := sum / float64(len(datapoints))

	currentVal := 0.0
	if latest.Average != nil {
		currentVal = *latest.Average
	}

	if converter != nil {
		currentVal = converter(currentVal)
		avg = converter(avg)
	}

	return formatMetricValue(currentVal), formatMetricValue(avg)
}

// formatMetricValue formats a numeric value for display.
func formatMetricValue(v float64) string {
	if v == 0 {
		return "0.00"
	}
	if v < 0.01 {
		return fmt.Sprintf("%.4f", v)
	}
	if v >= 1000 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.2f", v)
}

func init() {
	metricsCmd.Flags().BoolVar(&cloudMetrics, "cloud", false, "Fetch metrics from cloud (AWS CloudWatch)")
	rootCmd.AddCommand(metricsCmd)
}
