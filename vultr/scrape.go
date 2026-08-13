// Package vultr scrapes Vultr cloud compute plans.
package vultr

import (
	"encoding/json"
	"fmt"
	"github.com/fuzzbuster/ec2instances-cli/utils"
	"log"
	"sort"
)

const (
	outputFilePath = "vultr/instances.json"
	plansURL       = "https://api.vultr.com/v2/plans?per_page=500"
	regionsURL     = "https://api.vultr.com/v2/regions?per_page=500"
)

type PlanResponse struct {
	Plans []Plan `json:"plans"`
	Meta  Meta   `json:"meta"`
}

type RegionResponse struct {
	Regions []Region `json:"regions"`
	Meta    Meta     `json:"meta"`
}

type Meta struct {
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

type Plan struct {
	ID                string   `json:"id"`
	VCPUCount         int      `json:"vcpu_count"`
	RAM               int      `json:"ram"`
	Disk              int      `json:"disk"`
	DiskType          string   `json:"disk_type"`
	DiskCount         int      `json:"disk_count"`
	Bandwidth         int      `json:"bandwidth"`
	MonthlyCost       float64  `json:"monthly_cost"`
	HourlyCost        float64  `json:"hourly_cost"`
	Type              string   `json:"type"`
	Locations         []string `json:"locations"`
	CPUVendor         string   `json:"cpu_vendor"`
	StorageType       string   `json:"storage_type"`
	GPUVRAM           int      `json:"gpu_vram"`
	GPUType           string   `json:"gpu_type"`
	GPUBrand          string   `json:"gpu_brand"`
	DeployOnDemand    bool     `json:"deploy_ondemand"`
	DeployPreemptible bool     `json:"deploy_preemptible"`
	LocationCost      map[string]struct {
		MonthlyCost float64 `json:"monthly_cost"`
		HourlyCost  float64 `json:"hourly_cost"`
	} `json:"location_cost"`
}

type Region struct {
	ID      string `json:"id"`
	City    string `json:"city"`
	Country string `json:"country"`
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
	GPUMemory          int                                      `json:"GPU_memory,omitempty"`
	Storage            VPSStorage                               `json:"storage"`
	NetworkPerformance string                                   `json:"network_performance"`
	Transfer           int                                      `json:"transfer"`
	Pricing            map[string]map[string]map[string]float64 `json:"pricing"`
	Regions            map[string]string                        `json:"regions"`
	Availability       utils.Availability                       `json:"availability,omitempty"`
}

func DoVultrScraping() error {
	regions, err := fetchRegions()
	if err != nil {
		return err
	}
	plans, err := fetchPlans()
	if err != nil {
		return err
	}
	instances := make([]VPSInstance, 0, len(plans))

	for _, plan := range plans {
		if len(plan.Locations) == 0 {
			continue
		}
		instance := VPSInstance{
			Provider:           "vultr",
			InstanceType:       plan.ID,
			InstanceFamily:     plan.Type,
			PrettyName:         fmt.Sprintf("Vultr %s", plan.ID),
			VCPU:               plan.VCPUCount,
			Memory:             float64(plan.RAM) / 1024.0,
			Arch:               []string{"x86_64"},
			GPU:                gpuCount(plan),
			GPUModel:           gpuModel(plan),
			GPUMemory:          plan.GPUVRAM,
			Storage:            VPSStorage{Size: plan.Disk, SizeUnit: "GB", DiskType: plan.DiskType, DiskCount: plan.DiskCount},
			NetworkPerformance: networkPerformance(plan.Bandwidth),
			Transfer:           plan.Bandwidth,
			Pricing:            map[string]map[string]map[string]float64{},
			Regions:            map[string]string{},
			Availability:       utils.Availability{},
		}

		for _, region := range plan.Locations {
			hourlyCost := plan.HourlyCost
			if hourlyCost == 0 && plan.MonthlyCost > 0 {
				hourlyCost = plan.MonthlyCost / 730.0
			}
			if cost, ok := plan.LocationCost[region]; ok {
				if cost.HourlyCost > 0 {
					hourlyCost = cost.HourlyCost
				} else if cost.MonthlyCost > 0 {
					hourlyCost = cost.MonthlyCost / 730.0
				}
			}
			instance.Pricing[region] = map[string]map[string]float64{
				"linux": {"ondemand": hourlyCost},
			}
			instance.Regions[region] = regionName(region, regions)
			if options := vultrPurchaseOptions(plan); len(options) > 0 {
				instance.Availability[region] = utils.RegionAvailability{
					Status:          utils.AvailabilityAvailable,
					Evidence:        utils.AvailabilityCatalog,
					PurchaseOptions: options,
				}
			}
		}

		instances = append(instances, instance)
	}

	sort.Slice(instances, func(i, j int) bool {
		return instances[i].InstanceType < instances[j].InstanceType
	})

	if err := utils.SaveInstances(instances, outputFilePath); err != nil {
		return fmt.Errorf("save Vultr instances: %w", err)
	}
	log.Printf("[vultr] wrote %d plans", len(instances))
	return nil
}

func fetchPlans() ([]Plan, error) {
	var all []Plan
	nextURL := plansURL
	for nextURL != "" {
		body, err := utils.FetchWithRetry(nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("fetch Vultr plans: %w", err)
		}

		var response PlanResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("parse Vultr plans: %w", err)
		}
		all = append(all, response.Plans...)
		nextURL = nextPage(plansURL, response.Meta.Links.Next)
	}
	return all, nil
}

func fetchRegions() (map[string]Region, error) {
	regions := map[string]Region{}
	nextURL := regionsURL
	for nextURL != "" {
		body, err := utils.FetchWithRetry(nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("fetch Vultr regions: %w", err)
		}

		var response RegionResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("parse Vultr regions: %w", err)
		}
		for _, region := range response.Regions {
			regions[region.ID] = region
		}
		nextURL = nextPage(regionsURL, response.Meta.Links.Next)
	}
	return regions, nil
}

func nextPage(baseURL string, cursor string) string {
	if cursor == "" {
		return ""
	}
	return baseURL + "&cursor=" + cursor
}

func regionName(id string, regions map[string]Region) string {
	region, ok := regions[id]
	if !ok {
		return id
	}
	return fmt.Sprintf("%s, %s", region.City, region.Country)
}

func vultrPurchaseOptions(plan Plan) map[string]utils.AvailabilityStatus {
	options := map[string]utils.AvailabilityStatus{}
	if plan.DeployOnDemand {
		options["ondemand"] = utils.AvailabilityAvailable
	}
	if plan.DeployPreemptible {
		options["preemptible"] = utils.AvailabilityAvailable
	}
	return options
}

func networkPerformance(bandwidth int) string {
	if bandwidth <= 0 {
		return "Unknown"
	}
	return fmt.Sprintf("%d GB transfer", bandwidth)
}

func gpuCount(plan Plan) int {
	if plan.GPUType == "" && (plan.GPUBrand == "" || plan.GPUBrand == "none") && plan.GPUVRAM == 0 {
		return 0
	}
	return 1
}

func gpuModel(plan Plan) string {
	if plan.GPUType != "" {
		return plan.GPUType
	}
	if plan.GPUBrand != "" && plan.GPUBrand != "none" {
		return plan.GPUBrand
	}
	return ""
}
