package linode

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fuzzbuster/ec2instances-cli/utils"
)

func TestFetchPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Query().Get("page") {
		case "":
			fmt.Fprint(writer, `{"data":[{"id":"first"}],"page":1,"pages":2}`)
		case "2":
			fmt.Fprint(writer, `{"data":[{"id":"second"}],"page":2,"pages":2}`)
		default:
			http.Error(writer, "unexpected page", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	got, err := fetchPages[Type](server.URL+"?page_size=1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "first" || got[1].ID != "second" {
		t.Fatalf("fetchPages() = %+v", got)
	}
}

func TestRegionSupportsType(t *testing.T) {
	region := Region{Capabilities: []string{
		"Linodes",
		"GPU Linodes",
		"Premium Plans",
		"NETINT Quadra T1U",
	}}
	for _, class := range []string{"standard", "gpu", "premium", "accelerated"} {
		if !regionSupportsType(region, Type{Class: class}) {
			t.Errorf("region should support class %q", class)
		}
	}

	standardOnly := Region{Capabilities: []string{"Linodes"}}
	for _, class := range []string{"gpu", "premium", "accelerated"} {
		if regionSupportsType(standardOnly, Type{Class: class}) {
			t.Errorf("standard region unexpectedly supports class %q", class)
		}
	}
}

func TestLinodeRegionAvailability(t *testing.T) {
	availability := linodeRegionAvailability()
	if availability.Status != utils.AvailabilityAvailable ||
		availability.Evidence != utils.AvailabilityCatalog ||
		availability.PurchaseOptions["ondemand"] != utils.AvailabilityAvailable {
		t.Fatalf("availability = %+v", availability)
	}
}

func TestNetworkPerformance(t *testing.T) {
	tests := map[int]string{
		0:    "Unknown",
		500:  "500 Mbps",
		1000: "1 Gbps",
		2500: "2.5 Gbps",
	}
	for mbits, want := range tests {
		if got := networkPerformance(mbits); got != want {
			t.Errorf("networkPerformance(%d) = %q, want %q", mbits, got, want)
		}
	}
}

func TestGPUModel(t *testing.T) {
	if got := gpuModel(Type{GPUs: 1}); got != "GPU" {
		t.Fatalf("gpuModel() = %q, want GPU", got)
	}
	if got := gpuModel(Type{}); got != "" {
		t.Fatalf("non-GPU model = %q, want empty", got)
	}
}
