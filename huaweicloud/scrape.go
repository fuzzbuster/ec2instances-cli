// Package huaweicloud scrapes Huawei Cloud ECS instance type (flavor) data.
//
// Data source strategy:
//
//  1. Static seed data for all well-known flavor families (s, c, m, x, pi, p series).
//     Spec data: https://support.huaweicloud.com/intl/en-us/productdesc-ecs/ecs_01_0014.html
//
//  2. When HUAWEICLOUD_ACCESS_KEY / HUAWEICLOUD_SECRET_KEY and region-scoped
//     project IDs are set, the ECS ListFlavors API is called per region.
//     API reference: https://support.huaweicloud.com/intl/en-us/api-ecs/en-us_topic_0020212656.html
//
// No credentials are required to produce output — static seed covers GA flavors.
package huaweicloud

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/fuzzbuster/ec2instances-cli/utils"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	outputFilePath       = "huaweicloud/instances.json"
	portalMenuURL        = "https://portal.huaweicloud.com/api/calculator/rest/cbc/portalcalculatornodeservice/v4/api/menuInfo?sign=common&language=zh-cn"
	portalProductURLBase = "https://portal.huaweicloud.com/api/calculator/rest/cbc/portalcalculatornodeservice/v4/api/productInfo?urlPath=ecs&tag=general.online.portal&region="
)

// Huawei Cloud regions.
var huaweiRegions = []string{
	"cn-north-4",     // Beijing 4
	"cn-east-3",      // Shanghai 1
	"cn-south-1",     // Guangzhou
	"cn-east-2",      // East 2 (Shanghai 2)
	"cn-north-1",     // North 1 (Beijing 1)
	"cn-southwest-2", // Guizhou
	"ap-southeast-1", // Hong Kong
	"ap-southeast-2", // Bangkok
	"ap-southeast-3", // Singapore
	"na-mexico-1",    // Mexico
}

// ----- output instance struct -----

// HWInstance represents a Huawei Cloud ECS flavor.
type HWInstance struct {
	InstanceType   string             `json:"instance_type"`
	InstanceFamily string             `json:"instance_family"`
	PrettyName     string             `json:"pretty_name"`
	VCPU           int                `json:"vCPU"`
	Memory         float64            `json:"memory"` // GiB
	Arch           []string           `json:"arch"`
	GPU            int                `json:"GPU"`
	GPUModel       string             `json:"GPU_model,omitempty"`
	LocalStorage   string             `json:"local_storage,omitempty"`
	Availability   utils.Availability `json:"availability,omitempty"`
	// Pricing: region → os → {"ondemand": "price"}
	Pricing map[string]map[string]map[string]string `json:"pricing"`
	Regions []string                                `json:"regions"`
}

// ----- static seed data -----
// Source: https://support.huaweicloud.com/intl/en-us/productdesc-ecs/ecs_01_0014.html
type seed struct {
	InstanceType string
	VCPU         int
	MemGiB       float64
	Family       string
	PrettyName   string
	Arch         []string
	GPU          int
	GPUModel     string
}

var staticSeeds = []seed{
	// General Purpose s3 series
	{"s3.small.1", 1, 1, "s3", "General Purpose S3", []string{"x86_64"}, 0, ""},
	{"s3.medium.2", 1, 2, "s3", "General Purpose S3", []string{"x86_64"}, 0, ""},
	{"s3.large.2", 2, 4, "s3", "General Purpose S3", []string{"x86_64"}, 0, ""},
	{"s3.large.4", 2, 8, "s3", "General Purpose S3", []string{"x86_64"}, 0, ""},
	{"s3.xlarge.2", 4, 8, "s3", "General Purpose S3", []string{"x86_64"}, 0, ""},
	{"s3.xlarge.4", 4, 16, "s3", "General Purpose S3", []string{"x86_64"}, 0, ""},
	{"s3.2xlarge.2", 8, 16, "s3", "General Purpose S3", []string{"x86_64"}, 0, ""},
	{"s3.2xlarge.4", 8, 32, "s3", "General Purpose S3", []string{"x86_64"}, 0, ""},
	{"s3.4xlarge.2", 16, 32, "s3", "General Purpose S3", []string{"x86_64"}, 0, ""},
	{"s3.4xlarge.4", 16, 64, "s3", "General Purpose S3", []string{"x86_64"}, 0, ""},
	// General Purpose s6 series
	{"s6.small.1", 1, 1, "s6", "General Purpose S6", []string{"x86_64"}, 0, ""},
	{"s6.medium.2", 1, 2, "s6", "General Purpose S6", []string{"x86_64"}, 0, ""},
	{"s6.large.2", 2, 4, "s6", "General Purpose S6", []string{"x86_64"}, 0, ""},
	{"s6.large.4", 2, 8, "s6", "General Purpose S6", []string{"x86_64"}, 0, ""},
	{"s6.xlarge.2", 4, 8, "s6", "General Purpose S6", []string{"x86_64"}, 0, ""},
	{"s6.xlarge.4", 4, 16, "s6", "General Purpose S6", []string{"x86_64"}, 0, ""},
	{"s6.2xlarge.2", 8, 16, "s6", "General Purpose S6", []string{"x86_64"}, 0, ""},
	{"s6.2xlarge.4", 8, 32, "s6", "General Purpose S6", []string{"x86_64"}, 0, ""},
	{"s6.4xlarge.2", 16, 32, "s6", "General Purpose S6", []string{"x86_64"}, 0, ""},
	{"s6.4xlarge.4", 16, 64, "s6", "General Purpose S6", []string{"x86_64"}, 0, ""},
	// General Purpose Network-enhanced n3 series
	{"n3.large.2", 2, 4, "n3", "General Purpose Network-Enhanced N3", []string{"x86_64"}, 0, ""},
	{"n3.xlarge.2", 4, 8, "n3", "General Purpose Network-Enhanced N3", []string{"x86_64"}, 0, ""},
	{"n3.xlarge.4", 4, 16, "n3", "General Purpose Network-Enhanced N3", []string{"x86_64"}, 0, ""},
	{"n3.2xlarge.2", 8, 16, "n3", "General Purpose Network-Enhanced N3", []string{"x86_64"}, 0, ""},
	{"n3.2xlarge.4", 8, 32, "n3", "General Purpose Network-Enhanced N3", []string{"x86_64"}, 0, ""},
	{"n3.4xlarge.2", 16, 32, "n3", "General Purpose Network-Enhanced N3", []string{"x86_64"}, 0, ""},
	{"n3.4xlarge.4", 16, 64, "n3", "General Purpose Network-Enhanced N3", []string{"x86_64"}, 0, ""},
	{"n3.8xlarge.2", 32, 64, "n3", "General Purpose Network-Enhanced N3", []string{"x86_64"}, 0, ""},
	{"n3.8xlarge.4", 32, 128, "n3", "General Purpose Network-Enhanced N3", []string{"x86_64"}, 0, ""},
	// Compute Optimized c3 series
	{"c3.large.2", 2, 4, "c3", "Compute Optimized C3", []string{"x86_64"}, 0, ""},
	{"c3.xlarge.2", 4, 8, "c3", "Compute Optimized C3", []string{"x86_64"}, 0, ""},
	{"c3.xlarge.4", 4, 16, "c3", "Compute Optimized C3", []string{"x86_64"}, 0, ""},
	{"c3.2xlarge.2", 8, 16, "c3", "Compute Optimized C3", []string{"x86_64"}, 0, ""},
	{"c3.2xlarge.4", 8, 32, "c3", "Compute Optimized C3", []string{"x86_64"}, 0, ""},
	{"c3.4xlarge.2", 16, 32, "c3", "Compute Optimized C3", []string{"x86_64"}, 0, ""},
	{"c3.4xlarge.4", 16, 64, "c3", "Compute Optimized C3", []string{"x86_64"}, 0, ""},
	{"c3.8xlarge.2", 32, 64, "c3", "Compute Optimized C3", []string{"x86_64"}, 0, ""},
	// Memory Optimized m3 series
	{"m3.large.8", 2, 16, "m3", "Memory Optimized M3", []string{"x86_64"}, 0, ""},
	{"m3.xlarge.8", 4, 32, "m3", "Memory Optimized M3", []string{"x86_64"}, 0, ""},
	{"m3.2xlarge.8", 8, 64, "m3", "Memory Optimized M3", []string{"x86_64"}, 0, ""},
	{"m3.4xlarge.8", 16, 128, "m3", "Memory Optimized M3", []string{"x86_64"}, 0, ""},
	{"m3.8xlarge.8", 32, 256, "m3", "Memory Optimized M3", []string{"x86_64"}, 0, ""},
	// Disk-intensive d3 series
	{"d3.xlarge.4", 4, 16, "d3", "Disk-Intensive D3", []string{"x86_64"}, 0, ""},
	{"d3.2xlarge.4", 8, 32, "d3", "Disk-Intensive D3", []string{"x86_64"}, 0, ""},
	{"d3.4xlarge.4", 16, 64, "d3", "Disk-Intensive D3", []string{"x86_64"}, 0, ""},
	{"d3.8xlarge.4", 32, 128, "d3", "Disk-Intensive D3", []string{"x86_64"}, 0, ""},
	// GPU pi2 series (NVIDIA V100)
	{"pi2.2xlarge.4", 8, 32, "pi2", "GPU P2 (NVIDIA V100)", []string{"x86_64"}, 1, "NVIDIA V100"},
	{"pi2.8xlarge.4", 32, 128, "pi2", "GPU P2 (NVIDIA V100)", []string{"x86_64"}, 4, "NVIDIA V100"},
	// GPU p2s series (NVIDIA V100 32GB)
	{"p2s.2xlarge.8", 8, 64, "p2s", "GPU P2S (NVIDIA V100 32G)", []string{"x86_64"}, 1, "NVIDIA V100 32GB"},
	{"p2s.8xlarge.8", 32, 256, "p2s", "GPU P2S (NVIDIA V100 32G)", []string{"x86_64"}, 4, "NVIDIA V100 32GB"},
	// Arm Kunpeng kc1 series
	{"kc1.large.2", 2, 4, "kc1", "General Purpose Kunpeng KC1", []string{"arm64"}, 0, ""},
	{"kc1.xlarge.2", 4, 8, "kc1", "General Purpose Kunpeng KC1", []string{"arm64"}, 0, ""},
	{"kc1.xlarge.4", 4, 16, "kc1", "General Purpose Kunpeng KC1", []string{"arm64"}, 0, ""},
	{"kc1.2xlarge.2", 8, 16, "kc1", "General Purpose Kunpeng KC1", []string{"arm64"}, 0, ""},
	{"kc1.2xlarge.4", 8, 32, "kc1", "General Purpose Kunpeng KC1", []string{"arm64"}, 0, ""},
	{"kc1.4xlarge.2", 16, 32, "kc1", "General Purpose Kunpeng KC1", []string{"arm64"}, 0, ""},
	{"kc1.4xlarge.4", 16, 64, "kc1", "General Purpose Kunpeng KC1", []string{"arm64"}, 0, ""},
	{"kc1.8xlarge.2", 32, 64, "kc1", "General Purpose Kunpeng KC1", []string{"arm64"}, 0, ""},
	{"kc1.8xlarge.4", 32, 128, "kc1", "General Purpose Kunpeng KC1", []string{"arm64"}, 0, ""},
	// Arm Kunpeng Compute-optimised km1
	{"km1.large.2", 2, 4, "km1", "Memory Optimized Kunpeng KM1", []string{"arm64"}, 0, ""},
	{"km1.xlarge.4", 4, 16, "km1", "Memory Optimized Kunpeng KM1", []string{"arm64"}, 0, ""},
	{"km1.2xlarge.8", 8, 64, "km1", "Memory Optimized Kunpeng KM1", []string{"arm64"}, 0, ""},
}

// ----- API structs -----

type hwFlavor struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	VCPU         string `json:"vcpus"` // string in HW API
	RAM          int    `json:"ram"`   // MiB
	OsExtraSpecs struct {
		EcsInstanceArchitecture string `json:"ecs:instance_architecture"`
		EcsPerformanceType      string `json:"ecs:performancetype"`
		EcsGenerations          string `json:"ecs:generations"`
		GPU                     string `json:"pci_passthrough:count"`
		GPUSpec                 string `json:"pci_passthrough:alias"`
	} `json:"os_extra_specs"`
}

type hwFlavorsResponse struct {
	Flavors []hwFlavor `json:"flavors"`
}

// ----- anonymous pricing calculator fetch -----

type portalMenuItem struct {
	URLPath      string `json:"urlPath"`
	RegionOnline struct {
		RegionList []string `json:"regionList"`
	} `json:"regionOnline"`
	SubCategoryLists []portalMenuItem `json:"subCategoryLists"`
}

type portalMenuResponse struct {
	MenuInfos []portalMenuItem `json:"menuInfos"`
}

type portalPlan struct {
	BillingMode string  `json:"billingMode"`
	PeriodNum   *int    `json:"periodNum"`
	Amount      float64 `json:"amount"`
}

type portalVM struct {
	ResourceSpecCode string       `json:"resourceSpecCode"`
	CPU              string       `json:"cpu"`
	Memory           string       `json:"mem"`
	InstanceArch     string       `json:"instanceArch"`
	AcceleratorCard  string       `json:"acceleratorCard"`
	LocalDisk        string       `json:"localDisk"`
	Generation       string       `json:"generation"`
	Spec             string       `json:"spec"`
	ImageSpec        string       `json:"imageSpec"`
	PlanList         []portalPlan `json:"planList"`
}

type portalProductResponse struct {
	Region  string `json:"region"`
	Product struct {
		VMs []portalVM `json:"ec2_vm"`
	} `json:"product"`
}

func fetchPortalJSON(url string, dest any) error {
	resp, err := utils.RetryingHTTPClient().R().
		SetHeaders(map[string]string{
			"Accept":  "application/json",
			"Origin":  "https://www.huaweicloud.com",
			"Referer": "https://www.huaweicloud.com/pricing/calculator.html",
		}).
		Get(url)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	return json.Unmarshal(resp.Bytes(), dest)
}

func fetchPortalRegions() ([]string, error) {
	var response portalMenuResponse
	if err := fetchPortalJSON(portalMenuURL, &response); err != nil {
		return nil, err
	}
	regions := findPortalRegions(response.MenuInfos, "ecs")
	if len(regions) > 0 {
		sort.Strings(regions)
		return regions, nil
	}
	return nil, fmt.Errorf("ECS calculator menu not found")
}

func findPortalRegions(items []portalMenuItem, urlPath string) []string {
	for _, item := range items {
		if item.URLPath == urlPath {
			return append([]string(nil), item.RegionOnline.RegionList...)
		}
		if regions := findPortalRegions(item.SubCategoryLists, urlPath); len(regions) > 0 {
			return regions
		}
	}
	return nil
}

func fetchSpecsFromPortal() (map[string]*HWInstance, error) {
	regions, err := fetchPortalRegions()
	if err != nil {
		return nil, err
	}

	all := map[string]*HWInstance{}
	successfulRegions := 0
	for _, region := range regions {
		var response portalProductResponse
		url := portalProductURLBase + region + "&tab=calc&sign=common"
		if err := fetchPortalJSON(url, &response); err != nil {
			log.Printf("[huaweicloud] WARN anonymous region %s: %v", region, err)
			continue
		}
		successfulRegions++
		for _, vm := range response.Product.VMs {
			mergePortalVM(all, region, vm)
		}
		log.Printf("[huaweicloud] anonymous region %s: %d VM SKUs", region, len(response.Product.VMs))
	}
	if successfulRegions == 0 || len(all) == 0 {
		return nil, fmt.Errorf("anonymous calculator returned no instances from %d successful regions", successfulRegions)
	}
	return all, nil
}

func mergePortalVM(all map[string]*HWInstance, region string, vm portalVM) {
	if vm.ImageSpec != "linux" || vm.Spec == "" {
		return
	}
	instance := all[vm.Spec]
	if instance == nil {
		family := extractFamily(vm.Spec)
		gpuCount, gpuModel := parseAccelerator(vm.AcceleratorCard)
		instance = &HWInstance{
			InstanceType:   vm.Spec,
			InstanceFamily: family,
			PrettyName:     prettyName(family),
			VCPU:           parseLeadingInt(vm.CPU),
			Memory:         parseLeadingFloat(vm.Memory),
			Arch:           portalArch(vm.InstanceArch),
			GPU:            gpuCount,
			GPUModel:       gpuModel,
			LocalStorage:   normalizeLocalDisk(vm.LocalDisk),
			Pricing:        map[string]map[string]map[string]string{},
			Availability:   utils.Availability{},
		}
		all[vm.Spec] = instance
	}
	instance.Regions = utils.AppendUnique(instance.Regions, region)
	prices := map[string]string{}
	for _, plan := range vm.PlanList {
		key := portalPriceKey(plan)
		if key != "" && plan.Amount > 0 {
			prices[key] = strconv.FormatFloat(plan.Amount, 'f', -1, 64)
		}
	}
	if len(prices) > 0 {
		instance.Pricing[region] = map[string]map[string]string{"linux": prices}
	}
	options := map[string]utils.AvailabilityStatus{}
	for _, plan := range vm.PlanList {
		if option := portalPurchaseOption(plan.BillingMode); option != "" {
			options[option] = utils.AvailabilityOffered
		}
	}
	if len(options) > 0 {
		instance.Availability[region] = utils.RegionAvailability{
			Status:          utils.AvailabilityOffered,
			Evidence:        utils.AvailabilityPricing,
			PurchaseOptions: options,
		}
	}
}

func portalPriceKey(plan portalPlan) string {
	switch plan.BillingMode {
	case "ONDEMAND":
		return "ondemand"
	case "MONTHLY":
		return "monthly"
	case "YEARLY":
		if plan.PeriodNum != nil {
			return fmt.Sprintf("yearly_%d", *plan.PeriodNum)
		}
	}
	return ""
}

func portalPurchaseOption(billingMode string) string {
	switch billingMode {
	case "ONDEMAND":
		return "ondemand"
	case "MONTHLY", "YEARLY":
		return "prepaid"
	case "RI":
		return "reserved"
	default:
		return ""
	}
}

func parseLeadingInt(value string) int {
	var n int
	fmt.Sscanf(value, "%d", &n)
	return n
}

func parseLeadingFloat(value string) float64 {
	var n float64
	fmt.Sscanf(value, "%f", &n)
	return n
}

func portalArch(value string) []string {
	if strings.Contains(strings.ToLower(value), "arm") {
		return []string{"arm64"}
	}
	return []string{"x86_64"}
}

func parseAccelerator(value string) (int, string) {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return 0, ""
	}
	count := parseLeadingInt(value)
	if count == 0 {
		count = 1
	}
	model := strings.TrimSpace(value)
	if idx := strings.Index(model, "*"); idx >= 0 {
		model = strings.TrimSpace(model[idx+1:])
	}
	if idx := strings.Index(model, "/"); idx >= 0 {
		model = strings.TrimSpace(model[:idx])
	}
	return count, strings.ReplaceAll(model, "NVIDIAT4", "NVIDIA T4")
}

func normalizeLocalDisk(value string) string {
	if value == "0" || value == "" {
		return ""
	}
	return value
}

// ----- optional API fetch -----

func fetchSpecsFromAPI(ak, sk string, projectIDs map[string]string) map[string]*HWInstance {
	all := map[string]*HWInstance{}
	for _, region := range huaweiRegions {
		projectID := projectIDs[region]
		if projectID == "" {
			continue
		}
		flavors, err := listFlavors(ak, sk, projectID, region)
		if err != nil {
			log.Printf("[huaweicloud] WARN region %s: %v", region, err)
			continue
		}
		for _, f := range flavors {
			itype := f.ID
			inst, exists := all[itype]
			if !exists {
				vcpu := parseVCPU(f.VCPU)
				family := extractFamily(f.Name)
				arch := []string{"x86_64"}
				if strings.Contains(f.OsExtraSpecs.EcsInstanceArchitecture, "arm") {
					arch = []string{"arm64"}
				}
				inst = &HWInstance{
					InstanceType:   itype,
					InstanceFamily: family,
					PrettyName:     prettyName(family),
					VCPU:           vcpu,
					Memory:         float64(f.RAM) / 1024.0,
					Arch:           arch,
					Pricing:        map[string]map[string]map[string]string{},
				}
				all[itype] = inst
			}
			inst.Regions = utils.AppendUnique(inst.Regions, region)
		}
		log.Printf("[huaweicloud] region %s: %d flavors", region, len(flavors))
	}
	return all
}

func listFlavors(ak, sk, projectID, region string) ([]hwFlavor, error) {
	endpoint := fmt.Sprintf("https://ecs.%s.myhuaweicloud.com/v1/%s/cloudservers/flavors", region, projectID)

	timestamp := time.Now().UTC()
	authHeader, sdkDate, err := hwSignRequest(ak, sk, "GET", endpoint, "", region, timestamp)
	if err != nil {
		return nil, err
	}

	var out hwFlavorsResponse
	resp, err := utils.RetryingHTTPClient().R().
		SetHeaders(map[string]string{
			"Authorization": authHeader,
			"X-Sdk-Date":    sdkDate,
			"Content-Type":  "application/json",
		}).
		SetSuccessResult(&out).
		Get(endpoint)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, endpoint)
	}
	return out.Flavors, nil
}

// hwSignRequest implements Huawei Cloud AKSK authentication (HMAC-SHA256).
// Reference: https://support.huaweicloud.com/intl/en-us/api-ecs/ecs_03_0010.html
func hwSignRequest(ak, sk, method, endpoint, body, region string, t time.Time) (string, string, error) {
	datetime := t.Format("20060102T150405Z")
	date := t.Format("20060102")

	payloadHash := hashSHA256(body)
	canonicalHeaders := fmt.Sprintf("host:%s\nx-sdk-date:%s\n", extractHost(endpoint), datetime)
	signedHeaders := "host;x-sdk-date"

	canonicalRequest := strings.Join([]string{
		method, extractPath(endpoint), "",
		canonicalHeaders, signedHeaders, payloadHash,
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/ecs/sdk_request", date, region)
	stringToSign := strings.Join([]string{
		"SDK-HMAC-SHA256", datetime, credentialScope, hashSHA256(canonicalRequest),
	}, "\n")

	signingKey := hmacSHA256Bytes(
		hmacSHA256Bytes(
			hmacSHA256Bytes(
				hmacSHA256Bytes([]byte(sk), date),
				region),
			"ecs"),
		"sdk_request")
	signature := hex.EncodeToString(hmacSHA256Bytes(signingKey, stringToSign))

	return fmt.Sprintf(
		"SDK-HMAC-SHA256 Access=%s, SignedHeaders=%s, Signature=%s",
		ak, signedHeaders, signature,
	), datetime, nil
}

func huaweiProjectIDEnv(region string) string {
	return "HUAWEICLOUD_PROJECT_ID_" + strings.ToUpper(strings.ReplaceAll(region, "-", "_"))
}

func collectHuaweiProjectIDs() map[string]string {
	projectIDs := map[string]string{}
	for _, region := range huaweiRegions {
		if projectID := os.Getenv(huaweiProjectIDEnv(region)); projectID != "" {
			projectIDs[region] = projectID
		}
	}

	projectID := os.Getenv("HUAWEICLOUD_PROJECT_ID")
	region := os.Getenv("HUAWEICLOUD_REGION")
	if projectID != "" && region != "" {
		projectIDs[region] = projectID
	}
	if projectID != "" && region == "" && len(projectIDs) == 0 {
		log.Println("[huaweicloud] HUAWEICLOUD_PROJECT_ID set without HUAWEICLOUD_REGION; skipping live API to avoid cross-region project reuse")
	}
	return projectIDs
}

func hashSHA256(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func hmacSHA256Bytes(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func extractHost(endpoint string) string {
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	if idx := strings.Index(endpoint, "/"); idx >= 0 {
		return endpoint[:idx]
	}
	return endpoint
}

func extractPath(endpoint string) string {
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	if idx := strings.Index(endpoint, "/"); idx >= 0 {
		return endpoint[idx:]
	}
	return "/"
}

func extractFamily(name string) string {
	// Huawei flavor names: s6.xlarge.4, c3.large.2, kc1.2xlarge.8
	// Family = everything before the first dot
	if idx := strings.Index(name, "."); idx >= 0 {
		return name[:idx]
	}
	return name
}

func parseVCPU(s string) int {
	n := 0
	fmt.Sscanf(s, "%d", &n)
	return n
}

func prettyName(family string) string {
	f := strings.ToLower(family)
	switch {
	case strings.HasPrefix(f, "kc"):
		return "General Purpose Kunpeng " + strings.ToUpper(family)
	case strings.HasPrefix(f, "km"):
		return "Memory Optimized Kunpeng " + strings.ToUpper(family)
	case strings.HasPrefix(f, "pi"), strings.HasPrefix(f, "p2"):
		return "GPU " + strings.ToUpper(family)
	case strings.HasPrefix(f, "s"):
		return "General Purpose " + strings.ToUpper(family)
	case strings.HasPrefix(f, "n"):
		return "General Purpose Network-Enhanced " + strings.ToUpper(family)
	case strings.HasPrefix(f, "c"):
		return "Compute Optimized " + strings.ToUpper(family)
	case strings.HasPrefix(f, "m"):
		return "Memory Optimized " + strings.ToUpper(family)
	case strings.HasPrefix(f, "d"):
		return "Disk-Intensive " + strings.ToUpper(family)
	}
	return "Huawei Cloud ECS " + strings.ToUpper(family)
}

// ----- main entry point -----

// DoHuaweicloudScraping is called from main.go.
func DoHuaweicloudScraping() error {
	log.Println("[huaweicloud] starting scrape")

	all, err := fetchSpecsFromPortal()
	if err != nil {
		log.Printf("[huaweicloud] anonymous calculator failed: %v — using static seed data", err)
		all = map[string]*HWInstance{}
		for _, s := range staticSeeds {
			all[s.InstanceType] = &HWInstance{
				InstanceType:   s.InstanceType,
				InstanceFamily: s.Family,
				PrettyName:     s.PrettyName,
				VCPU:           s.VCPU,
				Memory:         s.MemGiB,
				Arch:           s.Arch,
				GPU:            s.GPU,
				GPUModel:       s.GPUModel,
				Pricing:        map[string]map[string]map[string]string{},
				Regions:        []string{"cn-north-4", "cn-east-3"},
			}
		}
	} else {
		log.Printf("[huaweicloud] anonymous calculator returned %d instances", len(all))
	}

	ak := os.Getenv("HUAWEICLOUD_ACCESS_KEY")
	sk := os.Getenv("HUAWEICLOUD_SECRET_KEY")
	projectIDs := collectHuaweiProjectIDs()
	if ak != "" && sk != "" && len(projectIDs) > 0 {
		log.Println("[huaweicloud] credentials found, fetching live data …")
		apiInstances := fetchSpecsFromAPI(ak, sk, projectIDs)
		for k, v := range apiInstances {
			if existing := all[k]; existing != nil {
				existing.VCPU = v.VCPU
				existing.Memory = v.Memory
				existing.Arch = v.Arch
				for _, region := range v.Regions {
					existing.Regions = utils.AppendUnique(existing.Regions, region)
				}
				continue
			}
			all[k] = v
		}
	} else if ak != "" && sk != "" {
		log.Println("[huaweicloud] credentials found but no region-scoped project IDs configured")
	}

	sorted := make([]*HWInstance, 0, len(all))
	for _, v := range all {
		sorted = append(sorted, v)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].InstanceType < sorted[j].InstanceType
	})

	if err := utils.SaveInstances(sorted, outputFilePath); err != nil {
		return fmt.Errorf("save Huawei Cloud instances: %w", err)
	}
	log.Printf("[huaweicloud] wrote %d instances to %s", len(sorted), outputFilePath)
	return nil
}
