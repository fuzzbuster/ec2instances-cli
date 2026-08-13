// Package hetzner scrapes Hetzner Cloud plans from public website data.
package hetzner

import (
	"encoding/json"
	"fmt"
	"github.com/fuzzbuster/ec2instances-cli/utils"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/anaskhan96/soup"
)

const (
	outputFilePath = "hetzner/instances.json"
	priceAPIURL    = "https://website-price-api.hetzner.com/api/v1/products/%s"
)

var productPages = []struct {
	family string
	url    string
}{
	{"Cost Optimized", "https://www.hetzner.com/cloud/cost-optimized/"},
	{"Regular Performance", "https://www.hetzner.com/cloud/regular-performance/"},
	{"General Purpose", "https://www.hetzner.com/cloud/general-purpose/"},
}

type Plan struct {
	Name       string
	Family     string
	VCPU       int
	Memory     float64
	Storage    int
	Arch       string
	ProductKey string
}

type PriceResponse struct {
	Locations []PriceLocation `json:"locations"`
}

type PriceLocation struct {
	CountryCode string `json:"countryCode"`
	Active      bool   `json:"active"`
	Datacenter  string `json:"datacenter"`
	Prices      struct {
		Hourly struct {
			USD string `json:"USD"`
			EUR string `json:"EUR"`
		} `json:"hourly"`
		Monthly struct {
			USD string `json:"USD"`
			EUR string `json:"EUR"`
		} `json:"monthly"`
	} `json:"prices"`
}

type VPSStorage struct {
	Size      int    `json:"size"`
	SizeUnit  string `json:"size_unit"`
	DiskType  string `json:"disk_type"`
	DiskCount int    `json:"disk_count"`
}

type VPSInstance struct {
	Provider           string                                   `json:"provider"`
	InstanceType       string                                   `json:"instance_type"`
	InstanceFamily     string                                   `json:"instance_family"`
	PrettyName         string                                   `json:"pretty_name"`
	VCPU               int                                      `json:"vCPU"`
	Memory             float64                                  `json:"memory"`
	Arch               []string                                 `json:"arch"`
	GPU                int                                      `json:"GPU"`
	Storage            VPSStorage                               `json:"storage"`
	NetworkPerformance string                                   `json:"network_performance"`
	Transfer           int                                      `json:"transfer"`
	Pricing            map[string]map[string]map[string]float64 `json:"pricing"`
	Regions            map[string]string                        `json:"regions"`
	Availability       utils.Availability                       `json:"availability,omitempty"`
}

func DoHetznerScraping() error {
	plans, err := fetchPlans()
	if err != nil {
		return err
	}
	instances := make([]VPSInstance, 0, len(plans))

	for _, plan := range plans {
		instance := VPSInstance{
			Provider:           "hetzner",
			InstanceType:       plan.Name,
			InstanceFamily:     plan.Family,
			PrettyName:         fmt.Sprintf("Hetzner %s", plan.Name),
			VCPU:               plan.VCPU,
			Memory:             plan.Memory,
			Arch:               architecture(plan.Arch),
			GPU:                0,
			Storage:            VPSStorage{Size: plan.Storage, SizeUnit: "GB", DiskType: "NVMe SSD", DiskCount: 1},
			NetworkPerformance: "20 TB traffic included",
			Transfer:           20000,
			Pricing:            map[string]map[string]map[string]float64{},
			Regions:            map[string]string{},
			Availability:       utils.Availability{},
		}

		locations, err := fetchPrices(plan.ProductKey)
		if err != nil {
			return err
		}
		for _, location := range locations {
			if !location.Active {
				continue
			}
			hourly, err := hourlyPrice(location)
			if err != nil {
				return fmt.Errorf(
					"parse Hetzner price for %s in %s: %w",
					plan.ProductKey,
					location.Datacenter,
					err,
				)
			}
			regionID := strings.ToLower(location.Datacenter)
			instance.Pricing[regionID] = map[string]map[string]float64{
				"linux": {"ondemand": hourly},
			}
			instance.Regions[regionID] = fmt.Sprintf("%s, %s", location.Datacenter, strings.ToUpper(location.CountryCode))
			instance.Availability[regionID] = hetznerRegionAvailability()
		}

		instances = append(instances, instance)
	}

	sort.Slice(instances, func(i, j int) bool {
		return instances[i].InstanceType < instances[j].InstanceType
	})

	if err := utils.SaveInstances(instances, outputFilePath); err != nil {
		return fmt.Errorf("save Hetzner instances: %w", err)
	}
	log.Printf("[hetzner] wrote %d cloud plans", len(instances))
	return nil
}

func fetchPlans() ([]Plan, error) {
	plans := []Plan{}
	seen := map[string]struct{}{}

	for _, page := range productPages {
		body, err := utils.FetchWithRetry(page.url, nil)
		if err != nil {
			return nil, fmt.Errorf("fetch Hetzner plans from %s: %w", page.url, err)
		}
		for _, plan := range parsePlans(string(body), page.family) {
			if _, ok := seen[plan.Name]; ok {
				continue
			}
			seen[plan.Name] = struct{}{}
			plans = append(plans, plan)
		}
	}
	if len(plans) == 0 {
		return nil, fmt.Errorf("no Hetzner cloud plans found")
	}
	return plans, nil
}

func parsePlans(pageHTML string, family string) []Plan {
	plans := []Plan{}
	doc := soup.HTMLParse(pageHTML)
	if doc.Error != nil {
		return plans
	}

	for _, row := range doc.FindAll("div", "class", "cloud-matrix-table-row") {
		nameNode := row.Find("div", "class", "name-cell")
		cpuNode := row.Find("div", "class", "cpu-cell")
		ramNode := row.Find("div", "class", "ram-cell")
		driveNode := row.Find("div", "class", "drive-cell")
		if nameNode.Error != nil || cpuNode.Error != nil || ramNode.Error != nil || driveNode.Error != nil {
			continue
		}

		name := strings.TrimSpace(nameNode.FullText())
		vcpu := firstInt(cpuNode.FullText())
		ram := firstFloat(ramNode.FullText())
		storage := firstInt(driveNode.FullText())

		priceNode := row.Find("ho-price-container")
		if priceNode.Error != nil {
			continue
		}
		productKey := priceNode.Attrs()["product-key"]

		archNode := row.Find("div", "class", "arch-type-badge")
		arch := ""
		if archNode.Error == nil {
			arch = strings.TrimSpace(archNode.FullText())
		}

		if name == "" || vcpu == 0 || ram == 0 || storage == 0 || productKey == "" {
			continue
		}

		plans = append(plans, Plan{
			Name:       name,
			Family:     family,
			VCPU:       vcpu,
			Memory:     ram,
			Storage:    storage,
			Arch:       arch,
			ProductKey: productKey,
		})
	}
	return plans
}

func fetchPrices(productKey string) ([]PriceLocation, error) {
	url := fmt.Sprintf(priceAPIURL, productKey)
	body, err := utils.FetchWithRetry(url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch Hetzner prices for %s: %w", productKey, err)
	}

	var response PriceResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("parse Hetzner prices for %s: %w", productKey, err)
	}
	return response.Locations, nil
}

func hetznerRegionAvailability() utils.RegionAvailability {
	return utils.RegionAvailability{
		Status:   utils.AvailabilityOffered,
		Evidence: utils.AvailabilityPricing,
		PurchaseOptions: map[string]utils.AvailabilityStatus{
			"ondemand": utils.AvailabilityOffered,
		},
	}
}

func firstInt(value string) int {
	for start := 0; start < len(value); start++ {
		if value[start] < '0' || value[start] > '9' {
			continue
		}
		end := start + 1
		for end < len(value) && value[end] >= '0' && value[end] <= '9' {
			end++
		}
		result, _ := strconv.Atoi(value[start:end])
		return result
	}
	return 0
}

func firstFloat(value string) float64 {
	for start := 0; start < len(value); start++ {
		if value[start] < '0' || value[start] > '9' {
			continue
		}
		end := start + 1
		seenDot := false
		for end < len(value) {
			if value[end] >= '0' && value[end] <= '9' {
				end++
				continue
			}
			if value[end] == '.' && !seenDot {
				seenDot = true
				end++
				continue
			}
			break
		}
		result, _ := strconv.ParseFloat(value[start:end], 64)
		return result
	}
	return 0
}

func hourlyPrice(location PriceLocation) (float64, error) {
	hourly, hourlyErr := parsePrice(location.Prices.Hourly.USD)
	if hourlyErr == nil && hourly > 0 {
		return hourly, nil
	}

	monthly, monthlyErr := parsePrice(location.Prices.Monthly.USD)
	if monthlyErr != nil {
		return 0, fmt.Errorf(
			"invalid hourly price %q and monthly price %q: %w",
			location.Prices.Hourly.USD,
			location.Prices.Monthly.USD,
			monthlyErr,
		)
	}
	if monthly <= 0 {
		return 0, fmt.Errorf(
			"hourly and monthly prices must be positive: hourly=%q monthly=%q",
			location.Prices.Hourly.USD,
			location.Prices.Monthly.USD,
		)
	}
	return monthly / 730.0, nil
}

func parsePrice(value string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(value), 64)
}

func architecture(value string) []string {
	value = strings.ToLower(value)
	if strings.Contains(value, "arm") || strings.Contains(value, "ampere") {
		return []string{"arm64"}
	}
	return []string{"x86_64"}
}
