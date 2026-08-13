package vultr

import (
	"reflect"
	"testing"

	"github.com/fuzzbuster/ec2instances-cli/utils"
)

func TestNextPageUsesVultrCursor(t *testing.T) {
	if got := nextPage(plansURL, "next-token"); got != plansURL+"&cursor=next-token" {
		t.Fatalf("nextPage() = %q", got)
	}
	if got := nextPage(plansURL, ""); got != "" {
		t.Fatalf("nextPage() with empty cursor = %q, want empty", got)
	}
}

func TestRegionName(t *testing.T) {
	regions := map[string]Region{
		"ams": {ID: "ams", City: "Amsterdam", Country: "NL"},
	}
	if got := regionName("ams", regions); got != "Amsterdam, NL" {
		t.Fatalf("regionName() = %q", got)
	}
	if got := regionName("unknown", regions); got != "unknown" {
		t.Fatalf("unknown regionName() = %q", got)
	}
}

func TestGPUHelpers(t *testing.T) {
	plan := Plan{GPUType: "NVIDIA A100", GPUBrand: "NVIDIA", GPUVRAM: 80}
	if got := gpuCount(plan); got != 1 {
		t.Fatalf("gpuCount() = %d, want 1", got)
	}
	if got := gpuModel(plan); got != "NVIDIA A100" {
		t.Fatalf("gpuModel() = %q", got)
	}
	if got := gpuCount(Plan{GPUBrand: "none"}); got != 0 {
		t.Fatalf("non-GPU gpuCount() = %d, want 0", got)
	}
}

func TestVultrPurchaseOptions(t *testing.T) {
	got := vultrPurchaseOptions(Plan{DeployOnDemand: true, DeployPreemptible: true})
	want := map[string]utils.AvailabilityStatus{
		"ondemand":    utils.AvailabilityAvailable,
		"preemptible": utils.AvailabilityAvailable,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("vultrPurchaseOptions() = %v, want %v", got, want)
	}
	if got := vultrPurchaseOptions(Plan{}); len(got) != 0 {
		t.Fatalf("disabled deployment options = %v", got)
	}
}
