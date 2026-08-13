package azure

import (
	"testing"

	"github.com/fuzzbuster/ec2instances-cli/utils"
)

func TestSplitAzureOfferKey(t *testing.T) {
	instanceType, tier, ok := splitAzureOfferKey("linux-e16-4as-v4-standard")
	if !ok || instanceType != "e16-4as-v4" || tier != "standard" {
		t.Fatalf("splitAzureOfferKey() = %q, %q, %v", instanceType, tier, ok)
	}
}

func TestProcessSpecsDataWithoutManagementAPI(t *testing.T) {
	specs := utils.NewSlowBuildingMap[string, *AzureSpecsApiIteratorItem](
		func(func(map[string]*AzureSpecsApiIteratorItem)) error { return nil },
	)
	instances := map[string]*AzureInstance{}
	attributes := map[string]map[string]any{
		"linux-e16-4as-v4-standard": {
			"instanceName": "E16-4as v4",
			"series":       "easv4",
			"category":     "memoryoptimized",
			"cores":        float64(4),
			"ram":          float64(128),
			"diskSize":     float64(32),
		},
	}

	if err := processSpecsDataResult(instances, attributes, specs); err != nil {
		t.Fatal(err)
	}
	instance := instances["e16-4as-v4"]
	if instance == nil {
		t.Fatal("constrained-vCPU instance was not parsed")
	}
	if instance.Vcpu != 4 || instance.Memory != 128 || instance.Size != 32 {
		t.Fatalf("unexpected anonymous specification: %+v", instance)
	}
}

func TestAvailabilityFromAzurePricing(t *testing.T) {
	availability := availabilityFromAzurePricing(map[string]map[string]map[string]any{
		"west-us": {
			"linux": {
				"ondemand":    1.25,
				"spot_min":    0.3,
				"lowpriority": 0.4,
				"reserved":    map[string]any{"yrTerm1Standard.allUpfront": 0.8},
			},
		},
	})
	region := availability["west-us"]
	if region.Status != utils.AvailabilityOffered || region.Evidence != utils.AvailabilityPricing {
		t.Fatalf("availability = %+v", region)
	}
	for _, option := range []string{"ondemand", "spot", "lowpriority", "reserved"} {
		if region.PurchaseOptions[option] != utils.AvailabilityOffered {
			t.Errorf("purchase option %q = %q", option, region.PurchaseOptions[option])
		}
	}
}

func TestHasAzureCredentialsRequiresCompleteSet(t *testing.T) {
	for _, key := range []string{
		"AZURE_TENANT_ID",
		"AZURE_CLIENT_ID",
		"AZURE_CLIENT_SECRET",
		"AZURE_SUBSCRIPTION_ID",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("AZURE_TENANT_ID", "tenant")
	if hasAzureCredentials() {
		t.Fatal("partial Azure credentials were accepted")
	}

	t.Setenv("AZURE_CLIENT_ID", "client")
	t.Setenv("AZURE_CLIENT_SECRET", "secret")
	t.Setenv("AZURE_SUBSCRIPTION_ID", "subscription")
	if !hasAzureCredentials() {
		t.Fatal("complete Azure credentials were rejected")
	}
}

func TestSpecsIteratorWithoutCredentialsCompletes(t *testing.T) {
	for _, key := range []string{
		"AZURE_TENANT_ID",
		"AZURE_CLIENT_ID",
		"AZURE_CLIENT_SECRET",
		"AZURE_SUBSCRIPTION_ID",
	} {
		t.Setenv(key, "")
	}
	if err := getAzureSpecsApiIterator().Wait(); err != nil {
		t.Fatalf("anonymous specs iterator returned error: %v", err)
	}
}
