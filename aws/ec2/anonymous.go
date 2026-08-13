package ec2

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/anaskhan96/soup"
	"github.com/fuzzbuster/ec2instances-cli/aws/awsutils"
	"github.com/fuzzbuster/ec2instances-cli/utils"
)

const anonymousOutputFile = "instances.json"

var acceleratorCountPattern = regexp.MustCompile(`^([0-9]+)\s*x\s*(.+)$`)

type compactPrice struct {
	Price string `json:"price"`
}

type compactPricingResponse struct {
	Regions map[string]map[string]compactPrice `json:"regions"`
}

// ProcessAnonymousData builds the global EC2 dataset from the compact public
// pricing map and the official instance-type specification documents.
func ProcessAnonymousData(
	documents [][]byte,
	pricingData []byte,
	apiDescriptions map[string]*APIInstanceTypeInfo,
) error {
	instances := map[string]*EC2Instance{}
	for _, document := range documents {
		if err := parseSpecificationDocument(document, instances); err != nil {
			return err
		}
	}
	if len(instances) == 0 {
		return fmt.Errorf("EC2 specification documents contained no instances")
	}

	var pricing compactPricingResponse
	if err := json.Unmarshal(pricingData, &pricing); err != nil {
		return fmt.Errorf("decode compact EC2 pricing: %w", err)
	}
	applyCompactPricing(instances, pricing)
	for instanceType, description := range apiDescriptions {
		if instance := instances[instanceType]; instance != nil {
			applyAPIInstanceDescription(instance, description)
		}
	}

	addGpuInfo(instances)
	addFpgaInfo(instances)
	addPlacementGroupInfo(instances)
	addLinuxAmiInfo(instances)
	addVpcOnlyInstances(instances)
	addDateIntroduced(instances)

	sorted := make([]*EC2Instance, 0, len(instances))
	for _, instance := range instances {
		sorted = append(sorted, instance)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].InstanceType < sorted[j].InstanceType
	})
	if err := utils.SaveInstances(sorted, anonymousOutputFile); err != nil {
		return fmt.Errorf("save anonymous EC2 instances: %w", err)
	}
	return nil
}

func parseSpecificationDocument(document []byte, instances map[string]*EC2Instance) error {
	root := soup.HTMLParse(string(document))
	for _, table := range root.FindAll("table") {
		headers := table.Find("thead").FindAll("th")
		if len(headers) < 4 || strings.TrimSpace(headers[0].Text()) != "Instance type" {
			continue
		}
		index := make(map[string]int, len(headers))
		for i, header := range headers {
			index[strings.TrimSpace(header.Text())] = i
		}
		if _, ok := index["Memory (GiB)"]; !ok {
			continue
		}
		if _, ok := index["vCPUs"]; !ok {
			continue
		}
		for _, row := range table.Find("tbody").FindAll("tr") {
			cells := row.FindAll("td")
			if len(cells) != len(headers) {
				continue
			}
			instanceType := cleanSpecificationValue(cells[index["Instance type"]].Text())
			if instanceType == "" || !strings.Contains(instanceType, ".") {
				continue
			}
			memory, err := strconv.ParseFloat(cleanSpecificationValue(cells[index["Memory (GiB)"]].Text()), 64)
			if err != nil {
				return fmt.Errorf("parse EC2 memory for %s: %w", instanceType, err)
			}
			vcpu, err := strconv.Atoi(cleanSpecificationValue(cells[index["vCPUs"]].Text()))
			if err != nil {
				return fmt.Errorf("parse EC2 vCPU for %s: %w", instanceType, err)
			}
			processor := cleanSpecificationValue(cells[index["Processor"]].Text())
			family := strings.SplitN(instanceType, ".", 2)[0]
			instance := &EC2Instance{
				InstanceType:             instanceType,
				Family:                   family,
				VCPU:                     awsutils.Averager[int]{vcpu},
				Memory:                   awsutils.Averager[float64]{memory},
				PrettyName:               awsutils.AddPrettyName(instanceType, EC2_FAMILY_NAMES),
				Arch:                     architectureForProcessor(processor),
				PhysicalProcessor:        processor,
				Generation:               "current",
				Pricing:                  map[Region]map[OS]any{},
				Regions:                  map[string]string{},
				LinuxVirtualizationTypes: []string{},
				VpcOnly:                  true,
				PlacementGroupSupport:    true,
				AvailabilityZones:        map[string][]string{},
				Availability:             utils.Availability{},
				IPV6Support:              true,
			}
			if coreIndex, ok := index["CPU cores"]; ok {
				cores, parseErr := strconv.Atoi(cleanSpecificationValue(cells[coreIndex].Text()))
				if parseErr == nil {
					instance.Cores = &cores
				}
			}
			if acceleratorIndex, ok := index["Accelerators"]; ok {
				setAccelerator(instance, cleanSpecificationValue(cells[acceleratorIndex].Text()))
			}
			instance.addExtraDetails()
			instances[instanceType] = instance
		}
	}
	return nil
}

func cleanSpecificationValue(value string) string {
	value = html.UnescapeString(value)
	value = strings.ReplaceAll(value, "\u2713", "")
	value = strings.ReplaceAll(value, "\u2717", "")
	return strings.TrimSpace(value)
}

func architectureForProcessor(processor string) []string {
	lower := strings.ToLower(processor)
	switch {
	case strings.Contains(lower, "graviton"):
		return []string{"arm64"}
	case strings.Contains(lower, "apple"):
		return []string{"arm64_mac"}
	default:
		return []string{"x86_64"}
	}
}

func setAccelerator(instance *EC2Instance, accelerator string) {
	lower := strings.ToLower(accelerator)
	if accelerator == "" || accelerator == "No" ||
		(!strings.Contains(lower, "nvidia") && !strings.Contains(lower, "amd")) {
		return
	}
	count := 1
	model := accelerator
	if matches := acceleratorCountPattern.FindStringSubmatch(accelerator); len(matches) == 3 {
		count, _ = strconv.Atoi(matches[1])
		model = strings.TrimSpace(matches[2])
	}
	instance.GPU = float64(count)
	instance.GPUModel = &model
}

func applyCompactPricing(instances map[string]*EC2Instance, pricing compactPricingResponse) {
	const keyPrefix = "OnDemand Linux-instancetype-"
	for location, entries := range pricing.Regions {
		for key, price := range entries {
			if !strings.HasPrefix(key, keyPrefix) || price.Price == "" {
				continue
			}
			instanceType := strings.TrimPrefix(key, keyPrefix)
			instance := instances[instanceType]
			if instance == nil {
				continue
			}
			instance.Pricing[location] = map[OS]any{
				"linux": &EC2PricingData{OnDemand: trimPrice(price.Price)},
			}
			instance.Regions[location] = location
			if instance.Availability == nil {
				instance.Availability = utils.Availability{}
			}
			instance.Availability[location] = utils.RegionAvailability{
				Status:   utils.AvailabilityOffered,
				Evidence: utils.AvailabilityPricing,
				PurchaseOptions: map[string]utils.AvailabilityStatus{
					"ondemand": utils.AvailabilityOffered,
				},
			}
		}
	}
}

func trimPrice(price string) string {
	value, err := strconv.ParseFloat(price, 64)
	if err != nil {
		return price
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}
