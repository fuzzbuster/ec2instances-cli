package tencentcloud

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/fuzzbuster/ec2instances-cli/utils"
)

func TestPostPublicJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if got := request.Header.Get("Content-Type"); got != "application/json; charset=UTF-8" {
			t.Errorf("Content-Type = %q", got)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != `{"region":"ap-nanjing"}` {
			t.Errorf("body = %q", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"code":0,"data":{"Response":{"RegionSet":[{"region":"ap-nanjing"}]}}}`)
	}))
	defer server.Close()

	var response publicRegionsResponse
	if err := postPublicJSON(server.URL, []byte(`{"region":"ap-nanjing"}`), &response); err != nil {
		t.Fatal(err)
	}
	if got := response.Data.Response.RegionSet[0].Region; got != "ap-nanjing" {
		t.Fatalf("region = %q, want ap-nanjing", got)
	}
}

func TestPostPublicJSONRejectsNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	var response publicRegionsResponse
	if err := postPublicJSON(server.URL, nil, &response); err == nil {
		t.Fatal("expected non-200 response to fail")
	}
}

func TestUniqueZones(t *testing.T) {
	zones := []publicZone{
		{Zone: "ap-nanjing-1"},
		{Zone: "ap-nanjing-1"},
		{Zone: ""},
		{Zone: "ap-nanjing-3"},
	}

	got := uniqueZones(zones)
	want := []string{"ap-nanjing-1", "ap-nanjing-3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("uniqueZones() = %v, want %v", got, want)
	}
}

func TestMergePublicInstance(t *testing.T) {
	instances := map[string]*CVMInstance{}
	base := publicInstance{
		InstanceType:      "SA5.MEDIUM8",
		InstanceFamily:    "SA5",
		TypeName:          "Standard Arm SA5",
		Architecture:      "ARM计算",
		CPU:               4,
		Memory:            8,
		InstanceBandwidth: 3,
		CPUType:           "Ampere Altra",
		LocalDiskTypeList: []publicLocalDisk{{
			Type:          "LOCAL_BASIC",
			PartitionType: "DATA",
			MaxSize:       500,
		}},
		Status: "SELL",
	}

	ondemand := base
	ondemand.InstanceChargeType = "POSTPAID_BY_HOUR"
	ondemand.Price.UnitPrice = 0.4
	ondemand.Price.UnitPriceDiscount = 0.3
	mergePublicInstance(instances, "ap-nanjing", "ap-nanjing-1", ondemand)

	lowerPrice := ondemand
	lowerPrice.Price.UnitPrice = 0.35
	lowerPrice.Price.UnitPriceDiscount = 0.25
	mergePublicInstance(instances, "ap-nanjing", "ap-nanjing-1", lowerPrice)

	prepaidAvailable := base
	prepaidAvailable.InstanceChargeType = "PREPAID"
	prepaidAvailable.Price.OriginalPrice = 180
	prepaidAvailable.Price.DiscountPrice = 150
	mergePublicInstance(instances, "ap-nanjing", "ap-nanjing-1", prepaidAvailable)

	spotSoldOut := base
	spotSoldOut.InstanceChargeType = "SPOTPAID"
	spotSoldOut.Status = "SOLD_OUT"
	spotSoldOut.Price.UnitPriceDiscount = 0.1
	mergePublicInstance(instances, "ap-nanjing", "ap-nanjing-1", spotSoldOut)

	prepaidSoldOut := prepaidAvailable
	prepaidSoldOut.Price.OriginalPrice = 200
	prepaidSoldOut.Price.DiscountPrice = 160
	prepaidSoldOut.Status = "SOLD_OUT"
	mergePublicInstance(instances, "ap-shanghai", "ap-shanghai-2", prepaidSoldOut)

	instance := instances[base.InstanceType]
	if instance == nil {
		t.Fatal("instance was not created")
	}
	if !reflect.DeepEqual(instance.Regions, []string{"ap-nanjing", "ap-shanghai"}) {
		t.Fatalf("regions = %v", instance.Regions)
	}
	if !reflect.DeepEqual(instance.Arch, []string{"arm64"}) {
		t.Fatalf("arch = %v, want arm64", instance.Arch)
	}
	if instance.NetworkPerformance != "3 Gbps" {
		t.Fatalf("network performance = %q", instance.NetworkPerformance)
	}
	if instance.PhysicalProcessor != "Ampere Altra" || instance.LocalStorage != "DATA LOCAL_BASIC 500 GiB" {
		t.Fatalf("hardware details = %q/%q", instance.PhysicalProcessor, instance.LocalStorage)
	}
	if !reflect.DeepEqual(instance.AvailabilityZones, map[string][]string{"ap-nanjing": {"ap-nanjing-1"}}) {
		t.Fatalf("availability zones = %v", instance.AvailabilityZones)
	}
	if got := instance.Pricing["ap-nanjing"]["linux"]["ondemand"]; got != "0.35" {
		t.Fatalf("ondemand price = %q, want 0.35", got)
	}
	if got := instance.Pricing["ap-nanjing"]["linux"]["ondemand_discount"]; got != "0.25" {
		t.Fatalf("discount price = %q, want 0.25", got)
	}
	if got := instance.Pricing["ap-shanghai"]["linux"]["monthly"]; got != "200" {
		t.Fatalf("monthly price = %q, want 200", got)
	}
	nanjing := instance.Availability["ap-nanjing"]
	if nanjing.Status != utils.AvailabilityAvailable || nanjing.Evidence != utils.AvailabilityRealtime {
		t.Fatalf("nanjing availability = %+v", nanjing)
	}
	wantOptions := map[string]utils.AvailabilityStatus{
		"ondemand": utils.AvailabilityAvailable,
		"prepaid":  utils.AvailabilityAvailable,
		"spot":     utils.AvailabilitySoldOut,
	}
	if !reflect.DeepEqual(nanjing.PurchaseOptions, wantOptions) {
		t.Fatalf("nanjing purchase options = %v, want %v", nanjing.PurchaseOptions, wantOptions)
	}
	nanjingZone := nanjing.Zones["ap-nanjing-1"]
	if nanjingZone.Status != utils.AvailabilityAvailable || !reflect.DeepEqual(nanjingZone.PurchaseOptions, wantOptions) {
		t.Fatalf("nanjing zone availability = %+v", nanjingZone)
	}
	shanghai := instance.Availability["ap-shanghai"]
	if shanghai.Status != utils.AvailabilitySoldOut ||
		shanghai.PurchaseOptions["prepaid"] != utils.AvailabilitySoldOut {
		t.Fatalf("shanghai availability = %+v", shanghai)
	}
}

func TestMergePublicInstanceIgnoresMissingType(t *testing.T) {
	instances := map[string]*CVMInstance{}
	mergePublicInstance(instances, "ap-nanjing", "ap-nanjing-1", publicInstance{})
	if len(instances) != 0 {
		t.Fatalf("missing instance type produced output: %v", instances)
	}
}

func TestMergePublicInstancePreservesUnknownStatus(t *testing.T) {
	instances := map[string]*CVMInstance{}
	mergePublicInstance(instances, "ap-nanjing", "ap-nanjing-3", publicInstance{
		InstanceType:       "S5.MEDIUM2",
		InstanceChargeType: "POSTPAID_BY_HOUR",
		Status:             "MAINTENANCE",
	})

	availability := instances["S5.MEDIUM2"].Availability["ap-nanjing"]
	if availability.Status != utils.AvailabilityUnknown ||
		availability.PurchaseOptions["ondemand"] != utils.AvailabilityUnknown {
		t.Fatalf("availability = %+v", availability)
	}
	if len(instances["S5.MEDIUM2"].AvailabilityZones) != 0 {
		t.Fatalf("unknown status marked sellable: %v", instances["S5.MEDIUM2"].AvailabilityZones)
	}
}
