package volcengine

import (
	"reflect"
	"testing"

	"github.com/fuzzbuster/ec2instances-cli/utils"
)

func TestMergeAnonymousSpecAndPrice(t *testing.T) {
	instances := map[string]*VEInstance{}
	spec := map[string]any{
		"Product":           "ECS",
		"ConfigurationCode": "ecs.g3i.large.month",
		"Region":            "cn-beijing",
		"AvailableZone":     `[{"ZoneCode":"cn-beijing-a"},{"ZoneCode":"cn-beijing-b"}]`,
		"BillingMethod":     "monthly",
		"PayType":           "pre",
		"AbilityAttrs": []any{
			map[string]any{"Ability": "vcpu", "Value": "2核"},
			map[string]any{"Ability": "memory", "Value": "8GiB"},
			map[string]any{"Ability": "processor", "Value": "Intel Xeon Platinum"},
			map[string]any{"Ability": "ssd", "Value": "3.84TB*1 SSD"},
		},
	}
	mergeAnonymousSpec(instances, spec)
	spec["ConfigurationCode"] = "ecs.g3i.large"
	spec["BillingMethod"] = "configuration_hourly"
	spec["PayType"] = "post"
	mergeAnonymousSpec(instances, spec)

	price := map[string]any{
		"Product":           "ECS",
		"ConfigurationCode": "ecs.g3i.large.month",
		"ChargeItemCode":    "ecs.g3i_large_cn-beijing",
		"PriceInfoList": []any{
			map[string]any{"Period": "monthly", "Times": float64(1), "Price": "253.38"},
			map[string]any{"Period": "monthly", "Times": float64(12), "Price": "1915.55"},
		},
	}
	mergeAnonymousPrice(instances, price)

	instance := instances["ecs.g3i.large"]
	if instance == nil {
		t.Fatal("instance was not added")
	}
	if instance.VCPU != 2 || instance.Memory != 8 {
		t.Fatalf("unexpected spec: vCPU=%d memory=%v", instance.VCPU, instance.Memory)
	}
	if instance.PhysicalProcessor != "Intel Xeon Platinum" || instance.LocalStorage != "3.84TB*1 SSD" {
		t.Fatalf("unexpected hardware fields: %+v", instance)
	}
	if len(instance.AvailabilityZones["cn-beijing"]) != 2 {
		t.Fatalf("unexpected availability zones: %v", instance.AvailabilityZones)
	}
	availability := instance.Availability["cn-beijing"]
	if availability.Status != utils.AvailabilityAvailable ||
		availability.Evidence != utils.AvailabilityCatalog ||
		availability.PurchaseOptions["ondemand"] != utils.AvailabilityAvailable ||
		availability.PurchaseOptions["prepaid"] != utils.AvailabilityAvailable {
		t.Fatalf("unexpected availability: %+v", availability)
	}
	for _, zone := range []string{"cn-beijing-a", "cn-beijing-b"} {
		if len(availability.Zones[zone].PurchaseOptions) != 2 {
			t.Fatalf("unexpected zone availability for %s: %+v", zone, availability.Zones[zone])
		}
	}
	prices := instance.Pricing["cn-beijing"]["linux"]
	if prices["monthly"] != "253.38" || prices["yearly_1"] != "1915.55" {
		t.Fatalf("unexpected prices: %v", prices)
	}
}

func TestMergeAnonymousPriceHourly(t *testing.T) {
	instances := map[string]*VEInstance{}
	mergeAnonymousSpec(instances, map[string]any{
		"Product":           "ECS",
		"ConfigurationCode": "ecs.c3a.large",
		"Region":            "cn-shanghai",
		"AbilityAttrs": []any{
			map[string]any{"Ability": "vcpu", "Value": "2核"},
			map[string]any{"Ability": "memory", "Value": "4GiB"},
		},
	})
	mergeAnonymousPrice(instances, map[string]any{
		"Product":           "ECS",
		"ConfigurationCode": "ecs.c3a.large",
		"ChargeItemCode":    "ecs.c3a_large_cn-shanghai",
		"PriceInfoList": []any{
			map[string]any{"Period": "hourly", "Times": float64(1), "Price": "0.42"},
		},
	})
	if got := instances["ecs.c3a.large"].Pricing["cn-shanghai"]["linux"]["ondemand"]; got != "0.42" {
		t.Fatalf("unexpected ondemand price %q", got)
	}
}

func TestAnonymousPriceKey(t *testing.T) {
	tests := []struct {
		period string
		times  int
		want   string
	}{
		{"hourly", 1, "ondemand"},
		{"monthly", 1, "monthly"},
		{"monthly", 12, "yearly_1"},
		{"monthly", 36, "yearly_3"},
		{"daily", 1, ""},
	}
	for _, test := range tests {
		if got := anonymousPriceKey(test.period, test.times); got != test.want {
			t.Errorf("anonymousPriceKey(%q, %d) = %q, want %q", test.period, test.times, got, test.want)
		}
	}
}

func TestDecodeAnonymousZones(t *testing.T) {
	got := decodeAnonymousZones(`[{"ZoneCode":"cn-beijing-a"},{"ZoneCode":""},{"ZoneCode":"cn-beijing-b"}]`)
	want := []string{"cn-beijing-a", "cn-beijing-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decodeAnonymousZones() = %v, want %v", got, want)
	}
	if got := decodeAnonymousZones(`not-json`); got != nil {
		t.Fatalf("invalid zones = %v, want nil", got)
	}
}
