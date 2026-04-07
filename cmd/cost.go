package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	cloudCost   bool
	compareCost bool
)

// pricingEntry holds the hardcoded cost for a specific resource type and tier.
type pricingEntry struct {
	Resource string
	Monthly  float64
}

// pricingTable maps cloud.type → tier → pricing.
// Approximate AWS us-east-1 monthly prices.
var pricingTable = map[string]map[string]pricingEntry{
	"postgres": {
		"free": {Resource: "RDS db.t3.micro", Monthly: 14.64},
		"standard": {Resource: "RDS db.t3.small", Monthly: 29.28},
	},
	"redis": {
		"free": {Resource: "ElastiCache t3.micro", Monthly: 12.24},
		"standard": {Resource: "ElastiCache t3.small", Monthly: 24.48},
	},
	"storage": {
		"free": {Resource: "S3 bucket", Monthly: 0.50},
		"standard": {Resource: "S3 bucket", Monthly: 0.50},
	},
}

var costCmd = &cobra.Command{
	Use:   "cost",
	Short: "Estimate cloud infrastructure costs",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !cloudCost {
			fmt.Println("Use stasis cost --cloud to see estimated cloud infrastructure costs.")
			fmt.Println("Add --compare to compare local vs cloud costs.")
			return nil
		}

		if compareCost {
			return runCostCompare()
		}
		return runCostEstimate()
	},
}

func runCostEstimate() error {
	fmt.Printf("\n💰 Stasis Cloud Cost Estimate\n")
	fmt.Printf("   Project: %s | Region: us-east-1\n\n", cfg.Name)

	separator := strings.Repeat("─", 75)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "   SERVICE\tTYPE\tTIER\tRESOURCE\t$/MONTH\n")
	fmt.Fprintf(w, "   %s\n", separator)

	var total float64
	found := false

	for name, service := range cfg.Services {
		if service.Cloud.Type == "" {
			continue
		}

		tier := service.Cloud.Tier
		if tier == "" {
			tier = "free"
		}

		entry, ok := lookupPricing(service.Cloud.Type, tier)
		if !ok {
			fmt.Fprintf(w, "   %s\t%s\t%s\t(unknown)\t—\n", name, service.Cloud.Type, tier)
			continue
		}

		fmt.Fprintf(w, "   %s\t%s\t%s\t%s\t$%.2f\n", name, service.Cloud.Type, tier, entry.Resource, entry.Monthly)
		total += entry.Monthly
		found = true
	}

	fmt.Fprintf(w, "   %s\n", separator)
	fmt.Fprintf(w, "   ESTIMATED TOTAL\t\t\t\t$%.2f\n", total)
	w.Flush()

	if !found {
		fmt.Println("\n   No services with cloud configuration found in stasis.yaml.")
	}

	fmt.Println("\n   ⚠️  Prices are estimates for us-east-1. Actual costs may vary.")
	fmt.Println()
	return nil
}

func runCostCompare() error {
	fmt.Printf("\n💰 Local vs Cloud Cost Comparison\n")
	fmt.Printf("   Project: %s\n\n", cfg.Name)

	separator := strings.Repeat("─", 70)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "   SERVICE\tTYPE\tLOCAL\tCLOUD\tSAVINGS\n")
	fmt.Fprintf(w, "   %s\n", separator)

	var totalCloud float64
	found := false

	for name, service := range cfg.Services {
		if service.Cloud.Type == "" {
			continue
		}

		tier := service.Cloud.Tier
		if tier == "" {
			tier = "free"
		}

		entry, ok := lookupPricing(service.Cloud.Type, tier)
		if !ok {
			fmt.Fprintf(w, "   %s\t%s\t$0.00\t—\t—\n", name, service.Cloud.Type)
			continue
		}

		fmt.Fprintf(w, "   %s\t%s\t$0.00\t$%.2f\t$%.2f\n",
			name, service.Cloud.Type, entry.Monthly, entry.Monthly)
		totalCloud += entry.Monthly
		found = true
	}

	fmt.Fprintf(w, "   %s\n", separator)
	fmt.Fprintf(w, "   TOTAL\t\t$0.00\t$%.2f\t$%.2f\n", totalCloud, totalCloud)
	w.Flush()

	if !found {
		fmt.Println("\n   No services with cloud configuration found in stasis.yaml.")
	} else {
		fmt.Printf("\n   ✅ Running locally saves you $%.2f/month\n", totalCloud)
	}

	fmt.Println()
	return nil
}

// lookupPricing returns the pricing entry for a given cloud type and tier.
func lookupPricing(cloudType, tier string) (pricingEntry, bool) {
	tiers, ok := pricingTable[cloudType]
	if !ok {
		return pricingEntry{}, false
	}
	entry, ok := tiers[tier]
	if !ok {
		// Fall back to "free" tier if the specified tier isn't found
		entry, ok = tiers["free"]
		if !ok {
			return pricingEntry{}, false
		}
	}
	return entry, true
}

func init() {
	costCmd.Flags().BoolVar(&cloudCost, "cloud", false, "Show estimated cloud infrastructure costs")
	costCmd.Flags().BoolVar(&compareCost, "compare", false, "Compare local vs cloud costs (use with --cloud)")
	rootCmd.AddCommand(costCmd)
}
