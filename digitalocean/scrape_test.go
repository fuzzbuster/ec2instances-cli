package digitalocean

import (
	"testing"

	"github.com/fuzzbuster/ec2instances-cli/utils"
)

func TestParsePlansFromPricingHTMLScriptPayload(t *testing.T) {
	pageHTML := `<html><body><script>
self.__next_f.push([1,"data"]);
{"memory":1,"cpus":1,"disk":{"boot":25},"network":{"throughput":2},"api":false,"price":{"transferQuota":1000,"hourly":0.00893,"monthly":6},"slug":"s-1vcpu-1gb"}
{"memory":2,"cpus":1,"disk":{"boot":50},"network":{"throughput":2},"price":{"transferQuota":2000,"hourly":0.01786,"monthly":12},"slug":"s-1vcpu-2gb"}
{"memory":2,"cpus":1,"disk":{"boot":50},"network":{"throughput":2},"price":{"transferQuota":2000,"hourly":0.01786,"monthly":12},"slug":"s-1vcpu-2gb"}
</script></body></html>`

	got, err := parsePlansFromPricingHTML(pageHTML)
	if err != nil {
		t.Fatalf("parsePlansFromPricingHTML returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("parsed %d plans, want 2: %+v", len(got), got)
	}

	if got[0].Slug != "s-1vcpu-1gb" || got[0].Memory != 1 || got[0].CPUs != 1 || got[0].Disk.Boot != 25 {
		t.Fatalf("first plan = %+v", got[0])
	}
	if got[1].Slug != "s-1vcpu-2gb" || got[1].Price.Hourly != 0.01786 || got[1].Price.TransferQuota != 2000 {
		t.Fatalf("second plan = %+v", got[1])
	}
}

func TestParsePlansFromPricingHTMLEscapedScriptPayload(t *testing.T) {
	pageHTML := `<html><body><script>
self.__next_f.push([1,"{\"memory\":4,\"cpus\":2,\"disk\":{\"boot\":80},\"network\":{\"throughput\":2},\"price\":{\"transferQuota\":4000,\"hourly\":0.03571,\"monthly\":24},\"slug\":\"s-2vcpu-4gb\"}"]);
</script></body></html>`

	got, err := parsePlansFromPricingHTML(pageHTML)
	if err != nil {
		t.Fatalf("parsePlansFromPricingHTML returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("parsed %d plans, want 1: %+v", len(got), got)
	}
	if got[0].Slug != "s-2vcpu-4gb" || got[0].Memory != 4 || got[0].CPUs != 2 {
		t.Fatalf("plan = %+v", got[0])
	}
}

func TestParsePlansFromPricingHTMLAllowsReorderedAndAdditionalFields(t *testing.T) {
	pageHTML := `<html><body><script>
{"slug":"s-2vcpu-4gb","extra":{"generation":2},"price":{"monthly":24,"hourly":0.03571,"transferQuota":4000},"network":{"throughput":2},"disk":{"type":"ssd","boot":80},"cpus":2,"memory":4}
</script></body></html>`

	got, err := parsePlansFromPricingHTML(pageHTML)
	if err != nil {
		t.Fatalf("parsePlansFromPricingHTML returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("parsed %d plans, want 1: %+v", len(got), got)
	}
	if got[0].Slug != "s-2vcpu-4gb" || got[0].Disk.Boot != 80 || got[0].Price.Hourly != 0.03571 {
		t.Fatalf("plan = %+v", got[0])
	}
}

func TestParsePlansFromPricingHTMLNoPlans(t *testing.T) {
	_, err := parsePlansFromPricingHTML(`<html><body><script>no plans here</script></body></html>`)
	if err == nil {
		t.Fatalf("expected error for page without plans")
	}
}

func TestInstanceFamily(t *testing.T) {
	tests := map[string]string{
		"g-4vcpu-16gb":  "General Purpose",
		"c-4":           "CPU-Optimized",
		"m-4vcpu-32gb":  "Memory-Optimized",
		"so-2vcpu-16gb": "Storage-Optimized",
		"s-1vcpu-1gb":   "Basic",
	}
	for slug, want := range tests {
		if got := instanceFamily(slug); got != want {
			t.Errorf("instanceFamily(%q) = %q, want %q", slug, got, want)
		}
	}
}

func TestParseDropletAvailability(t *testing.T) {
	markdown := `| Droplet Plans | NYC1 | ATL1 | FUT1 |
|---|---|---|---|
| Basic | ✓ | ◐ | |`
	got, err := parseDropletAvailability(markdown, map[string]string{
		"nyc1": "New York 1",
		"atl1": "Atlanta 1",
		"fut1": "Future 1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["Basic"]["nyc1"] != utils.AvailabilityAvailable {
		t.Fatalf("full availability = %q", got["Basic"]["nyc1"])
	}
	if got["Basic"]["atl1"] != utils.AvailabilityLimited {
		t.Fatalf("limited availability = %q", got["Basic"]["atl1"])
	}
	if _, ok := got["Basic"]["fut1"]; ok {
		t.Fatalf("future region was marked available: %v", got["Basic"])
	}
}

func TestNetworkPerformance(t *testing.T) {
	if got := networkPerformance(2.5); got != "2.5 Gbps" {
		t.Fatalf("networkPerformance() = %q", got)
	}
	if got := networkPerformance(0); got != "Unknown" {
		t.Fatalf("zero network performance = %q", got)
	}
}
