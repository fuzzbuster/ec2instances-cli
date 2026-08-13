package hetzner

import (
	"math"
	"testing"

	"github.com/fuzzbuster/ec2instances-cli/utils"
)

func TestParsePlansFromCloudMatrixRows(t *testing.T) {
	pageHTML := `
<div class="cloud-matrix-table-row">
	<div class="name-cell"><div class="">CX23</div></div>
	<div class="cpu-cell">
		<img src="/cpu.svg" alt="">
		2
		<div class="arch-type-badge">Ampere</div>
	</div>
	<div class="ram-cell"><img src="/ram.svg" alt="">4 GB</div>
	<div class="drive-cell"><img src="/ssd.svg" alt="">40 GB</div>
	<ho-price-container product-key="cx23"></ho-price-container>
	<div class="table-card">
		<div class="name-cell">SHOULD-NOT-WIN</div>
		<div class="data-cell-collection">
			<div class="cpu-cell">99</div>
			<div class="ram-cell">999 GB</div>
			<div class="drive-cell">999 GB</div>
		</div>
	</div>
</div>
<div class="cloud-matrix-table-row">
	<div class="name-cell"><div class="">BROKEN</div></div>
	<div class="cpu-cell">2</div>
	<div class="ram-cell">4 GB</div>
	<div class="drive-cell">40 GB</div>
</div>`

	got := parsePlans(pageHTML, "Cost Optimized")
	if len(got) != 1 {
		t.Fatalf("parsed %d plans, want 1: %+v", len(got), got)
	}

	want := Plan{
		Name:       "CX23",
		Family:     "Cost Optimized",
		VCPU:       2,
		Memory:     4,
		Storage:    40,
		Arch:       "Ampere",
		ProductKey: "cx23",
	}
	if got[0] != want {
		t.Fatalf("plan = %+v, want %+v", got[0], want)
	}
}

func TestParsePlansSkipsRowsWithMissingRequiredCells(t *testing.T) {
	pageHTML := `
<div class="cloud-matrix-table-row">
	<div class="name-cell">BROKEN</div>
	<div class="cpu-cell">2</div>
	<div class="ram-cell">4 GB</div>
	<ho-price-container product-key="broken"></ho-price-container>
</div>`

	if got := parsePlans(pageHTML, "Cost Optimized"); len(got) != 0 {
		t.Fatalf("parsed %d plans, want 0: %+v", len(got), got)
	}
}

func TestHourlyPriceFallsBackToMonthly(t *testing.T) {
	var location PriceLocation
	location.Prices.Hourly.USD = "not available"
	location.Prices.Monthly.USD = "73"

	got, err := hourlyPrice(location)
	if err != nil {
		t.Fatalf("hourlyPrice returned error: %v", err)
	}
	if math.Abs(got-0.1) > 1e-9 {
		t.Fatalf("hourly price = %v, want 0.1", got)
	}
}

func TestHourlyPriceRejectsInvalidPrices(t *testing.T) {
	var location PriceLocation
	location.Prices.Hourly.USD = "not available"
	location.Prices.Monthly.USD = "unknown"

	if _, err := hourlyPrice(location); err == nil {
		t.Fatal("expected invalid prices to return an error")
	}
}

func TestHetznerRegionAvailability(t *testing.T) {
	availability := hetznerRegionAvailability()
	if availability.Status != utils.AvailabilityOffered ||
		availability.Evidence != utils.AvailabilityPricing ||
		availability.PurchaseOptions["ondemand"] != utils.AvailabilityOffered {
		t.Fatalf("availability = %+v", availability)
	}
}

func TestArchitecture(t *testing.T) {
	for _, value := range []string{"ARM64", "Ampere Altra"} {
		got := architecture(value)
		if len(got) != 1 || got[0] != "arm64" {
			t.Errorf("architecture(%q) = %v, want arm64", value, got)
		}
	}
	got := architecture("AMD EPYC")
	if len(got) != 1 || got[0] != "x86_64" {
		t.Fatalf("architecture(AMD EPYC) = %v, want x86_64", got)
	}
}

func TestHourlyPricePrefersHourly(t *testing.T) {
	var location PriceLocation
	location.Prices.Hourly.USD = "0.012"
	location.Prices.Monthly.USD = "73"

	got, err := hourlyPrice(location)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.012 {
		t.Fatalf("hourly price = %v, want 0.012", got)
	}
}
