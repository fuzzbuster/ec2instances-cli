package ec2

import (
	"testing"

	"github.com/fuzzbuster/ec2instances-cli/utils"
)

func TestParseSpecificationDocument(t *testing.T) {
	document := []byte(`
<table>
<thead><tr><th>Instance type</th><th>Memory (GiB)</th><th>Processor</th><th>vCPUs</th><th>CPU cores</th><th>Threads per core</th><th>Accelerators</th><th>Accelerator memory</th></tr></thead>
<tbody>
<tr><td>m7g.large</td><td>8.00</td><td>AWS Graviton3</td><td>2</td><td>2</td><td>1</td><td>✗ No</td><td>✗ No</td></tr>
<tr><td>g6.xlarge</td><td>16.00</td><td>AMD EPYC 7R13</td><td>4</td><td>2</td><td>2</td><td>1 x NVIDIA L4</td><td>24 GB</td></tr>
</tbody>
</table>`)
	instances := map[string]*EC2Instance{}

	if err := parseSpecificationDocument(document, instances); err != nil {
		t.Fatal(err)
	}

	graviton := instances["m7g.large"]
	if graviton == nil {
		t.Fatal("m7g.large was not parsed")
	}
	if graviton.VCPU.Value() != 2 || graviton.Memory.Value() != 8 || graviton.Cores == nil || *graviton.Cores != 2 {
		t.Fatalf("unexpected Graviton spec: %+v", graviton)
	}
	if len(graviton.Arch) != 1 || graviton.Arch[0] != "arm64" {
		t.Fatalf("unexpected Graviton architecture %v", graviton.Arch)
	}

	gpu := instances["g6.xlarge"]
	if gpu == nil || gpu.GPU != 1 || gpu.GPUModel == nil || *gpu.GPUModel != "NVIDIA L4" {
		t.Fatalf("unexpected GPU spec: %+v", gpu)
	}
}

func TestApplyCompactPricing(t *testing.T) {
	instances := map[string]*EC2Instance{
		"m7g.large": {
			InstanceType: "m7g.large",
			Pricing:      map[Region]map[OS]any{},
			Regions:      map[string]string{},
		},
	}
	pricing := compactPricingResponse{
		Regions: map[string]map[string]compactPrice{
			"US East (N. Virginia)": {
				"OnDemand Linux-instancetype-m7g.large": {Price: "0.0816000000"},
				"OnDemand Linux-unused":                 {Price: "999"},
			},
		},
	}

	applyCompactPricing(instances, pricing)

	linux, ok := instances["m7g.large"].Pricing["US East (N. Virginia)"]["linux"].(*EC2PricingData)
	if !ok || linux.OnDemand != "0.0816" {
		t.Fatalf("unexpected Linux pricing: %#v", instances["m7g.large"].Pricing)
	}
	if instances["m7g.large"].Regions["US East (N. Virginia)"] != "US East (N. Virginia)" {
		t.Fatalf("unexpected regions: %v", instances["m7g.large"].Regions)
	}
	availability := instances["m7g.large"].Availability["US East (N. Virginia)"]
	if availability.Status != utils.AvailabilityOffered ||
		availability.Evidence != utils.AvailabilityPricing ||
		availability.PurchaseOptions["ondemand"] != utils.AvailabilityOffered {
		t.Fatalf("unexpected availability: %+v", availability)
	}
}

func TestParseSpecificationDocumentSkipsIncompleteRows(t *testing.T) {
	document := []byte(`
<table>
<thead><tr><th>Instance type</th><th>Memory (GiB)</th><th>Processor</th><th>vCPUs</th></tr></thead>
<tbody>
<tr><td>family heading</td><td></td><td></td><td></td></tr>
<tr><td>m7i.large</td><td>8.00</td><td>Intel Xeon</td></tr>
</tbody>
</table>`)
	instances := map[string]*EC2Instance{}

	if err := parseSpecificationDocument(document, instances); err != nil {
		t.Fatal(err)
	}
	if len(instances) != 0 {
		t.Fatalf("incomplete rows produced instances: %v", instances)
	}
}
