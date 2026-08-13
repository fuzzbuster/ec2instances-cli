package huaweicloud

import (
	"testing"

	"github.com/fuzzbuster/ec2instances-cli/utils"
)

func TestMergePortalVM(t *testing.T) {
	oneYear := 1
	instances := map[string]*HWInstance{}
	vm := portalVM{
		CPU:             "8BSSUNIT.pluralUnit.23",
		Memory:          "32BSSUNIT.pluralUnit.102",
		InstanceArch:    "x86",
		AcceleratorCard: "2 * NVIDIA T4 / 2 * 16G",
		LocalDisk:       "2*1.8T NVMe SSD",
		Spec:            "g6.2xlarge.4",
		ImageSpec:       "linux",
		PlanList: []portalPlan{
			{BillingMode: "ONDEMAND", Amount: 4.25},
			{BillingMode: "MONTHLY", PeriodNum: &oneYear, Amount: 2040},
			{BillingMode: "YEARLY", PeriodNum: &oneYear, Amount: 20400},
			{BillingMode: "RI", PeriodNum: &oneYear, Amount: 18000},
		},
	}

	mergePortalVM(instances, "cn-north-4", vm)
	mergePortalVM(instances, "cn-east-3", vm)

	instance := instances["g6.2xlarge.4"]
	if instance == nil {
		t.Fatal("instance was not added")
	}
	if instance.VCPU != 8 || instance.Memory != 32 {
		t.Fatalf("unexpected spec: vCPU=%d memory=%v", instance.VCPU, instance.Memory)
	}
	if instance.GPU != 2 || instance.GPUModel != "NVIDIA T4" {
		t.Fatalf("unexpected GPU: count=%d model=%q", instance.GPU, instance.GPUModel)
	}
	if instance.LocalStorage != "2*1.8T NVMe SSD" {
		t.Fatalf("unexpected local storage %q", instance.LocalStorage)
	}
	if len(instance.Regions) != 2 {
		t.Fatalf("unexpected regions %v", instance.Regions)
	}
	prices := instance.Pricing["cn-north-4"]["linux"]
	if prices["ondemand"] != "4.25" || prices["monthly"] != "2040" || prices["yearly_1"] != "20400" {
		t.Fatalf("unexpected prices %v", prices)
	}
	availability := instance.Availability["cn-north-4"]
	if availability.Status != utils.AvailabilityOffered ||
		availability.Evidence != utils.AvailabilityPricing {
		t.Fatalf("unexpected availability: %+v", availability)
	}
	for _, option := range []string{"ondemand", "prepaid", "reserved"} {
		if availability.PurchaseOptions[option] != utils.AvailabilityOffered {
			t.Errorf("purchase option %q = %q", option, availability.PurchaseOptions[option])
		}
	}
}

func TestFindPortalRegionsInNestedCategory(t *testing.T) {
	var ecs portalMenuItem
	ecs.URLPath = "ecs"
	ecs.RegionOnline.RegionList = []string{"cn-north-4", "cn-east-3"}
	items := []portalMenuItem{{SubCategoryLists: []portalMenuItem{ecs}}}

	regions := findPortalRegions(items, "ecs")
	if len(regions) != 2 || regions[0] != "cn-north-4" || regions[1] != "cn-east-3" {
		t.Fatalf("regions = %v", regions)
	}
	if regions := findPortalRegions(items, "missing"); regions != nil {
		t.Fatalf("missing category regions = %v", regions)
	}
}

func TestMergePortalVMIgnoresWindows(t *testing.T) {
	instances := map[string]*HWInstance{}
	mergePortalVM(instances, "cn-north-4", portalVM{
		Spec:      "s7.large.2",
		ImageSpec: "win",
	})
	if len(instances) != 0 {
		t.Fatalf("expected Windows SKU to be ignored, got %v", instances)
	}
}

func TestPortalPriceKey(t *testing.T) {
	threeYears := 3
	tests := []struct {
		plan portalPlan
		want string
	}{
		{portalPlan{BillingMode: "ONDEMAND"}, "ondemand"},
		{portalPlan{BillingMode: "MONTHLY"}, "monthly"},
		{portalPlan{BillingMode: "YEARLY", PeriodNum: &threeYears}, "yearly_3"},
		{portalPlan{BillingMode: "YEARLY"}, ""},
		{portalPlan{BillingMode: "UNKNOWN"}, ""},
	}
	for _, test := range tests {
		if got := portalPriceKey(test.plan); got != test.want {
			t.Errorf("portalPriceKey(%+v) = %q, want %q", test.plan, got, test.want)
		}
	}
}

func TestParseAccelerator(t *testing.T) {
	count, model := parseAccelerator("2 * NVIDIAT4 / 2 * 16G")
	if count != 2 || model != "NVIDIA T4" {
		t.Fatalf("parseAccelerator() = %d, %q", count, model)
	}
	count, model = parseAccelerator("")
	if count != 0 || model != "" {
		t.Fatalf("empty accelerator = %d, %q", count, model)
	}
}
