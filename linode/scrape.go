// Package linode scrapes Linode instance type data.
package linode

import (
	"encoding/json"
	"fmt"
	"github.com/fuzzbuster/ec2instances-cli/utils"
	"log"
	"sort"
	"strings"
)

const (
	outputFilePath = "linode/instances.json"
	typesURL       = "https://api.linode.com/v4/linode/types?page_size=500"
	regionsURL     = "https://api.linode.com/v4/regions?page_size=500"
)

type PageResponse[T any] struct {
	Data  []T `json:"data"`
	Page  int `json:"page"`
	Pages int `json:"pages"`
}

type Type struct {
	ID                 string        `json:"id"`
	Label              string        `json:"label"`
	Class              string        `json:"class"`
	Memory             int           `json:"memory"`
	Disk               int           `json:"disk"`
	Transfer           int           `json:"transfer"`
	VCPUs              int           `json:"vcpus"`
	GPUs               int           `json:"gpus"`
	NetworkOut         int           `json:"network_out"`
	Capabilities       []string      `json:"capabilities"`
	Price              Price         `json:"price"`
	RegionPrices       []RegionPrice `json:"region_prices"`
	AcceleratedDevices int           `json:"accelerated_devices"`
}

type Price struct {
	Hourly  float64 `json:"hourly"`
	Monthly float64 `json:"monthly"`
}

type RegionPrice struct {
	ID      string  `json:"id"`
	Hourly  float64 `json:"hourly"`
	Monthly float64 `json:"monthly"`
}

type Region struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Country      string   `json:"country"`
	Status       string   `json:"status"`
	SiteType     string   `json:"site_type"`
	Capabilities []string `json:"capabilities"`
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
	GPUModel           string                                   `json:"GPU_model,omitempty"`
	AcceleratedDevices int                                      `json:"accelerated_devices,omitempty"`
	Storage            VPSStorage                               `json:"storage"`
	NetworkPerformance string                                   `json:"network_performance"`
	Transfer           int                                      `json:"transfer"`
	Pricing            map[string]map[string]map[string]float64 `json:"pricing"`
	Regions            map[string]string                        `json:"regions"`
	Availability       utils.Availability                       `json:"availability,omitempty"`
}

func DoLinodeScraping() error {
	regions, err := fetchRegions()
	if err != nil {
		return err
	}
	types, err := fetchTypes()
	if err != nil {
		return err
	}
	instances := make([]VPSInstance, 0, len(types))

	for _, instanceType := range types {
		instance := VPSInstance{
			Provider:           "linode",
			InstanceType:       instanceType.ID,
			InstanceFamily:     instanceType.Class,
			PrettyName:         instanceType.Label,
			VCPU:               instanceType.VCPUs,
			Memory:             float64(instanceType.Memory) / 1024.0,
			Arch:               []string{"x86_64"},
			GPU:                instanceType.GPUs,
			GPUModel:           gpuModel(instanceType),
			AcceleratedDevices: instanceType.AcceleratedDevices,
			Storage:            VPSStorage{Size: instanceType.Disk / 1024, SizeUnit: "GB", DiskType: "SSD", DiskCount: 1},
			NetworkPerformance: networkPerformance(instanceType.NetworkOut),
			Transfer:           instanceType.Transfer,
			Pricing:            map[string]map[string]map[string]float64{},
			Regions:            map[string]string{},
			Availability:       utils.Availability{},
		}

		regionPrices := map[string]float64{}
		for _, regionPrice := range instanceType.RegionPrices {
			hourly := regionPrice.Hourly
			if hourly == 0 && regionPrice.Monthly > 0 {
				hourly = regionPrice.Monthly / 730.0
			}
			regionPrices[regionPrice.ID] = hourly
		}

		defaultHourly := instanceType.Price.Hourly
		if defaultHourly == 0 && instanceType.Price.Monthly > 0 {
			defaultHourly = instanceType.Price.Monthly / 730.0
		}

		for regionID, region := range regions {
			if !regionSupportsType(region, instanceType) {
				continue
			}
			hourly := defaultHourly
			if regionHourly, ok := regionPrices[regionID]; ok {
				hourly = regionHourly
			}
			instance.Pricing[regionID] = map[string]map[string]float64{
				"linux": {"ondemand": hourly},
			}
			instance.Regions[regionID] = region.Label
			instance.Availability[regionID] = linodeRegionAvailability()
		}

		instances = append(instances, instance)
	}

	sort.Slice(instances, func(i, j int) bool {
		return instances[i].InstanceType < instances[j].InstanceType
	})

	if err := utils.SaveInstances(instances, outputFilePath); err != nil {
		return fmt.Errorf("save Linode instances: %w", err)
	}
	log.Printf("[linode] wrote %d instance types", len(instances))
	return nil
}

func fetchTypes() ([]Type, error) {
	return fetchPages[Type](typesURL, "[linode] fetch types")
}

func fetchRegions() (map[string]Region, error) {
	all, err := fetchPages[Region](regionsURL, "[linode] fetch regions")
	if err != nil {
		return nil, err
	}
	regions := map[string]Region{}
	for _, region := range all {
		if region.Status != "ok" || !contains(region.Capabilities, "Linodes") {
			continue
		}
		regions[region.ID] = region
	}
	return regions, nil
}

func fetchPages[T any](baseURL string, logPrefix string) ([]T, error) {
	var all []T
	pageURL := baseURL
	for {
		body, err := utils.FetchWithRetry(pageURL, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", logPrefix, err)
		}

		var response PageResponse[T]
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("%s parse: %w", logPrefix, err)
		}
		all = append(all, response.Data...)

		if response.Page >= response.Pages {
			break
		}
		pageURL = fmt.Sprintf("%s&page=%d", baseURL, response.Page+1)
	}
	return all, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func linodeRegionAvailability() utils.RegionAvailability {
	return utils.RegionAvailability{
		Status:   utils.AvailabilityAvailable,
		Evidence: utils.AvailabilityCatalog,
		PurchaseOptions: map[string]utils.AvailabilityStatus{
			"ondemand": utils.AvailabilityAvailable,
		},
	}
}

func regionSupportsType(region Region, instanceType Type) bool {
	switch instanceType.Class {
	case "gpu":
		return contains(region.Capabilities, "GPU Linodes")
	case "premium":
		return contains(region.Capabilities, "Premium Plans")
	case "accelerated":
		return contains(region.Capabilities, "NETINT Quadra T1U")
	default:
		return true
	}
}

func networkPerformance(mbits int) string {
	if mbits <= 0 {
		return "Unknown"
	}
	if mbits >= 1000 {
		gbits := float64(mbits) / 1000.0
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", gbits), "0"), ".") + " Gbps"
	}
	return fmt.Sprintf("%d Mbps", mbits)
}

func gpuModel(instanceType Type) string {
	if instanceType.GPUs > 0 {
		return "GPU"
	}
	return ""
}
