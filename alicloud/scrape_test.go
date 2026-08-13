package alicloud

import (
	"testing"

	"github.com/fuzzbuster/ec2instances-cli/utils"
)

func TestBuildInstancesFromAnonymousPricing(t *testing.T) {
	intl := pricingFile{PricingInfo: map[string]pricingEntry{
		"us-east-1::ecs.g6.xlarge::vpc::linux::optimized": {
			Hours:  []pricePeriod{{Price: "0.20", Period: "1"}},
			Months: []pricePeriod{{Price: "100", Period: "1"}},
		},
	}}
	cn := pricingFile{PricingInfo: map[string]pricingEntry{
		"cn-hangzhou::ecs.g6.xlarge::vpc::linux::optimized": {
			Hours: []pricePeriod{{Price: "1.30", Period: "1"}},
		},
	}}

	instance := buildInstances(intl, cn, nil)["ecs.g6.xlarge"]
	if instance == nil {
		t.Fatal("instance was not created")
	}
	if instance.VCPU != 4 || instance.Memory != 16 {
		t.Fatalf("spec = %d vCPU/%v GiB, want 4/16", instance.VCPU, instance.Memory)
	}
	if got := instance.Pricing["us-east-1"]["linux"]["ondemand"]; got != "0.20" {
		t.Fatalf("international price = %q, want 0.20", got)
	}
	if got := instance.Pricing["cn-hangzhou"]["linux"]["ondemand"]; got != "1.30" {
		t.Fatalf("China price = %q, want 1.30", got)
	}
	if len(instance.Regions) != 2 {
		t.Fatalf("regions = %v, want two regions", instance.Regions)
	}
	availability := instance.Availability["us-east-1"]
	if availability.Status != utils.AvailabilityOffered ||
		availability.Evidence != utils.AvailabilityPricing ||
		availability.PurchaseOptions["ondemand"] != utils.AvailabilityOffered ||
		availability.PurchaseOptions["prepaid"] != utils.AvailabilityOffered {
		t.Fatalf("availability = %+v", availability)
	}
}

func TestBuildInstancesPrefersAPISpecification(t *testing.T) {
	apiSpec := &AliInstance{
		InstanceType:   "ecs.custom.large",
		InstanceFamily: "custom",
		VCPU:           6,
		Memory:         24,
		Arch:           []string{"arm64"},
	}
	pricing := pricingFile{PricingInfo: map[string]pricingEntry{
		"us-east-1::ecs.custom.large::vpc::linux::optimized": {
			Hours: []pricePeriod{{Price: "0.42", Period: "1"}},
		},
	}}

	instance := buildInstances(pricing, pricingFile{}, map[string]*AliInstance{
		apiSpec.InstanceType: apiSpec,
	})[apiSpec.InstanceType]
	if instance != apiSpec {
		t.Fatal("authoritative API specification was not reused")
	}
	if instance.VCPU != 6 || instance.Memory != 24 || instance.Arch[0] != "arm64" {
		t.Fatalf("API specification was changed: %+v", instance)
	}
	if got := instance.Pricing["us-east-1"]["linux"]["ondemand"]; got != "0.42" {
		t.Fatalf("price = %q, want 0.42", got)
	}
}

func TestGuessSpecHandlesEncodedRatios(t *testing.T) {
	vcpu, memory, ok := guessSpec("ecs.t5-c1m2.large")
	if !ok || vcpu != 2 || memory != 4 {
		t.Fatalf("guessSpec() = %d, %v, %v; want 2, 4, true", vcpu, memory, ok)
	}
}
