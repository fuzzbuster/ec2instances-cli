// Package volcengine scrapes Volcengine (ByteDance) ECS instance type data.
//
// Data source strategy:
//
//  1. Static seed data for all well-known instance families (g*, c*, r*, i*, gn*).
//     Spec data: https://www.volcengine.com/docs/6396/70840
//
//  2. When VOLCENGINE_ACCESS_KEY / VOLCENGINE_SECRET_KEY are set, the
//     DescribeInstanceTypes API is called for live region × instance data.
//     API reference: https://www.volcengine.com/docs/6396/71002
//
// No credentials are required to produce output — the static seed covers all
// GA instance types as of 2026-Q1.
package volcengine

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/fuzzbuster/ec2instances-cli/utils"
	"github.com/imroc/req/v3"
	"log"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	outputFilePath           = "volcengine/instances.json"
	ecsAPIHost               = "open.volcengineapi.com"
	ecsAPIRegion             = "cn-beijing" // service endpoint region
	ecsService               = "ecs"
	anonymousPriceURL        = "https://www.volcengine.com/anonymous-api/trade/price"
	anonymousECSTemplateCode = "CLT7310571628986405164"
)

// Volcengine regions list.
var volcengineRegions = []string{
	"cn-beijing", "cn-shanghai", "cn-guangzhou", "cn-chengdu",
	"ap-southeast-1", "ap-southeast-3", "ap-southeast-2",
	"us-east-1",
}

// ----- output instance struct -----

// VEInstance represents a Volcengine ECS instance type.
type VEInstance struct {
	InstanceType      string              `json:"instance_type"`
	InstanceFamily    string              `json:"instance_family"`
	PrettyName        string              `json:"pretty_name"`
	VCPU              int                 `json:"vCPU"`
	Memory            float64             `json:"memory"` // GiB
	Arch              []string            `json:"arch"`
	GPU               int                 `json:"GPU"`
	GPUModel          string              `json:"GPU_model,omitempty"`
	PhysicalProcessor string              `json:"physical_processor,omitempty"`
	LocalStorage      string              `json:"local_storage,omitempty"`
	AvailabilityZones map[string][]string `json:"availability_zones,omitempty"`
	Availability      utils.Availability  `json:"availability,omitempty"`
	// Pricing: region → os → {"ondemand": "price"}
	Pricing map[string]map[string]map[string]string `json:"pricing"`
	Regions []string                                `json:"regions"`
}

// ----- static seed data -----
// Source: https://www.volcengine.com/docs/6396/70840  (verified 2026-Q1)
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
	// General Purpose g series
	{"ecs.g1.small", 1, 2, "g1", "General Purpose G1", []string{"x86_64"}, 0, ""},
	{"ecs.g1.medium", 2, 4, "g1", "General Purpose G1", []string{"x86_64"}, 0, ""},
	{"ecs.g1.large", 4, 8, "g1", "General Purpose G1", []string{"x86_64"}, 0, ""},
	{"ecs.g1.xlarge", 8, 16, "g1", "General Purpose G1", []string{"x86_64"}, 0, ""},
	{"ecs.g1.2xlarge", 16, 32, "g1", "General Purpose G1", []string{"x86_64"}, 0, ""},
	{"ecs.g1.4xlarge", 32, 64, "g1", "General Purpose G1", []string{"x86_64"}, 0, ""},
	{"ecs.g1.8xlarge", 64, 128, "g1", "General Purpose G1", []string{"x86_64"}, 0, ""},
	{"ecs.g2.small", 1, 2, "g2", "General Purpose G2", []string{"x86_64"}, 0, ""},
	{"ecs.g2.medium", 2, 4, "g2", "General Purpose G2", []string{"x86_64"}, 0, ""},
	{"ecs.g2.large", 4, 8, "g2", "General Purpose G2", []string{"x86_64"}, 0, ""},
	{"ecs.g2.xlarge", 8, 16, "g2", "General Purpose G2", []string{"x86_64"}, 0, ""},
	{"ecs.g2.2xlarge", 16, 32, "g2", "General Purpose G2", []string{"x86_64"}, 0, ""},
	{"ecs.g2.4xlarge", 32, 64, "g2", "General Purpose G2", []string{"x86_64"}, 0, ""},
	{"ecs.g2.8xlarge", 64, 128, "g2", "General Purpose G2", []string{"x86_64"}, 0, ""},
	{"ecs.g3.small", 1, 2, "g3", "General Purpose G3", []string{"x86_64"}, 0, ""},
	{"ecs.g3.medium", 2, 4, "g3", "General Purpose G3", []string{"x86_64"}, 0, ""},
	{"ecs.g3.large", 4, 8, "g3", "General Purpose G3", []string{"x86_64"}, 0, ""},
	{"ecs.g3.xlarge", 8, 16, "g3", "General Purpose G3", []string{"x86_64"}, 0, ""},
	{"ecs.g3.2xlarge", 16, 32, "g3", "General Purpose G3", []string{"x86_64"}, 0, ""},
	{"ecs.g3.4xlarge", 32, 64, "g3", "General Purpose G3", []string{"x86_64"}, 0, ""},
	{"ecs.g3.8xlarge", 64, 128, "g3", "General Purpose G3", []string{"x86_64"}, 0, ""},
	// Compute Optimized c series
	{"ecs.c1.large", 4, 8, "c1", "Compute Optimized C1", []string{"x86_64"}, 0, ""},
	{"ecs.c1.xlarge", 8, 16, "c1", "Compute Optimized C1", []string{"x86_64"}, 0, ""},
	{"ecs.c1.2xlarge", 16, 32, "c1", "Compute Optimized C1", []string{"x86_64"}, 0, ""},
	{"ecs.c1.4xlarge", 32, 64, "c1", "Compute Optimized C1", []string{"x86_64"}, 0, ""},
	{"ecs.c1.8xlarge", 64, 128, "c1", "Compute Optimized C1", []string{"x86_64"}, 0, ""},
	{"ecs.c3.large", 4, 8, "c3", "Compute Optimized C3", []string{"x86_64"}, 0, ""},
	{"ecs.c3.xlarge", 8, 16, "c3", "Compute Optimized C3", []string{"x86_64"}, 0, ""},
	{"ecs.c3.2xlarge", 16, 32, "c3", "Compute Optimized C3", []string{"x86_64"}, 0, ""},
	{"ecs.c3.4xlarge", 32, 64, "c3", "Compute Optimized C3", []string{"x86_64"}, 0, ""},
	{"ecs.c3.8xlarge", 64, 128, "c3", "Compute Optimized C3", []string{"x86_64"}, 0, ""},
	// Memory Optimized r series
	{"ecs.r1.large", 4, 32, "r1", "Memory Optimized R1", []string{"x86_64"}, 0, ""},
	{"ecs.r1.xlarge", 8, 64, "r1", "Memory Optimized R1", []string{"x86_64"}, 0, ""},
	{"ecs.r1.2xlarge", 16, 128, "r1", "Memory Optimized R1", []string{"x86_64"}, 0, ""},
	{"ecs.r1.4xlarge", 32, 256, "r1", "Memory Optimized R1", []string{"x86_64"}, 0, ""},
	{"ecs.r3.large", 4, 32, "r3", "Memory Optimized R3", []string{"x86_64"}, 0, ""},
	{"ecs.r3.xlarge", 8, 64, "r3", "Memory Optimized R3", []string{"x86_64"}, 0, ""},
	{"ecs.r3.2xlarge", 16, 128, "r3", "Memory Optimized R3", []string{"x86_64"}, 0, ""},
	{"ecs.r3.4xlarge", 32, 256, "r3", "Memory Optimized R3", []string{"x86_64"}, 0, ""},
	// GPU gn series
	{"ecs.gn1l.xlarge", 4, 30, "gn1l", "GPU GN1L (NVIDIA T4)", []string{"x86_64"}, 1, "NVIDIA Tesla T4"},
	{"ecs.gn1l.4xlarge", 16, 120, "gn1l", "GPU GN1L (NVIDIA T4)", []string{"x86_64"}, 4, "NVIDIA Tesla T4"},
	{"ecs.gn1l.8xlarge", 32, 240, "gn1l", "GPU GN1L (NVIDIA T4)", []string{"x86_64"}, 8, "NVIDIA Tesla T4"},
	{"ecs.gn2l.xlarge", 8, 60, "gn2l", "GPU GN2L (NVIDIA A10)", []string{"x86_64"}, 1, "NVIDIA A10"},
	{"ecs.gn2l.4xlarge", 30, 240, "gn2l", "GPU GN2L (NVIDIA A10)", []string{"x86_64"}, 4, "NVIDIA A10"},
	{"ecs.gn2l.8xlarge", 60, 480, "gn2l", "GPU GN2L (NVIDIA A10)", []string{"x86_64"}, 8, "NVIDIA A10"},
	{"ecs.gn2o.xlarge", 8, 60, "gn2o", "GPU GN2O (NVIDIA A30)", []string{"x86_64"}, 1, "NVIDIA A30"},
	{"ecs.gn2o.4xlarge", 30, 240, "gn2o", "GPU GN2O (NVIDIA A30)", []string{"x86_64"}, 4, "NVIDIA A30"},
	// IO Optimized i series
	{"ecs.i2.xlarge", 4, 16, "i2", "Storage Optimized I2", []string{"x86_64"}, 0, ""},
	{"ecs.i2.2xlarge", 8, 32, "i2", "Storage Optimized I2", []string{"x86_64"}, 0, ""},
	{"ecs.i2.4xlarge", 16, 64, "i2", "Storage Optimized I2", []string{"x86_64"}, 0, ""},
	{"ecs.i2.8xlarge", 32, 128, "i2", "Storage Optimized I2", []string{"x86_64"}, 0, ""},
}

// ----- API structs -----

type veProcessor struct {
	Cpus *int32 `json:"Cpus"`
}

type veMemory struct {
	Size *int32 `json:"Size"` // MiB
}

type veInstanceType struct {
	InstanceTypeId     *string      `json:"InstanceTypeId"`
	InstanceTypeFamily *string      `json:"InstanceTypeFamily"`
	Processor          *veProcessor `json:"Processor"`
	Memory             *veMemory    `json:"Memory"`
	GPU                *struct {
		GPUDevices []struct {
			Count       *int32  `json:"Count"`
			ProductName *string `json:"ProductName"`
		} `json:"GPUDevices"`
	} `json:"GPU"`
}

type veDescribeResponse struct {
	ResponseMetadata struct {
		RequestId string `json:"RequestId"`
		Error     *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	} `json:"ResponseMetadata"`
	Result struct {
		InstanceTypes []veInstanceType `json:"InstanceTypes"`
		NextToken     string           `json:"NextToken"`
	} `json:"Result"`
}

// ----- anonymous pricing portal fetch -----

type anonymousTemplateResponse struct {
	Result struct {
		List []struct {
			Template string `json:"Template"`
		} `json:"List"`
	} `json:"Result"`
}

func fetchAnonymousSpecs() (map[string]*VEInstance, error) {
	templateURL := anonymousPriceURL + "?Action=ListTemplate&Version=2020-01-01&TemplateCode=" + anonymousECSTemplateCode
	var templateResponse anonymousTemplateResponse
	if err := fetchAnonymousJSON("GET", templateURL, nil, &templateResponse); err != nil {
		return nil, err
	}
	if len(templateResponse.Result.List) == 0 {
		return nil, fmt.Errorf("anonymous ECS template is empty")
	}

	var template any
	if err := json.Unmarshal([]byte(templateResponse.Result.List[0].Template), &template); err != nil {
		return nil, fmt.Errorf("decode anonymous ECS template: %w", err)
	}
	all := map[string]*VEInstance{}
	walkJSONObjects(template, func(object map[string]any) {
		mergeAnonymousSpec(all, object)
	})
	if len(all) == 0 {
		return nil, fmt.Errorf("anonymous ECS template contained no instance types")
	}

	tableURL := anonymousPriceURL + "?Action=GetTable&Version=2020-01-01"
	var tableResponse any
	body := map[string]string{"TemplateCode": anonymousECSTemplateCode}
	if err := fetchAnonymousJSON("POST", tableURL, body, &tableResponse); err != nil {
		return nil, err
	}
	walkJSONObjects(tableResponse, func(object map[string]any) {
		mergeAnonymousPrice(all, object)
	})
	return all, nil
}

func fetchAnonymousJSON(method, url string, body any, dest any) error {
	request := utils.RetryingHTTPClient().R().
		SetHeaders(map[string]string{
			"Accept":  "application/json",
			"Origin":  "https://www.volcengine.com",
			"Referer": "https://www.volcengine.com/pricing?product=ECS&tab=2",
		}).
		SetSuccessResult(dest)
	var (
		response *req.Response
		err      error
	)
	if method == "POST" {
		response, err = request.SetBody(body).Post(url)
	} else {
		response, err = request.Get(url)
	}
	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		return fmt.Errorf("HTTP %d for %s", response.StatusCode, url)
	}
	return nil
}

func walkJSONObjects(value any, visit func(map[string]any)) {
	switch current := value.(type) {
	case map[string]any:
		visit(current)
		for _, child := range current {
			walkJSONObjects(child, visit)
		}
	case []any:
		for _, child := range current {
			walkJSONObjects(child, visit)
		}
	}
}

func mergeAnonymousSpec(all map[string]*VEInstance, object map[string]any) {
	if stringValue(object["Product"]) != "ECS" {
		return
	}
	instanceType := normalizeAnonymousInstanceType(stringValue(object["ConfigurationCode"]))
	if instanceType == "" {
		return
	}
	abilities, ok := object["AbilityAttrs"].([]any)
	if !ok {
		return
	}

	instance := all[instanceType]
	if instance == nil {
		family := strings.SplitN(strings.TrimPrefix(instanceType, "ecs."), ".", 2)[0]
		instance = &VEInstance{
			InstanceType:      instanceType,
			InstanceFamily:    family,
			PrettyName:        prettyName(family),
			Arch:              []string{"x86_64"},
			Pricing:           map[string]map[string]map[string]string{},
			AvailabilityZones: map[string][]string{},
			Availability:      utils.Availability{},
		}
		all[instanceType] = instance
	}
	for _, rawAbility := range abilities {
		ability, ok := rawAbility.(map[string]any)
		if !ok {
			continue
		}
		value := stringValue(ability["Value"])
		switch stringValue(ability["Ability"]) {
		case "vcpu":
			instance.VCPU = leadingInt(value)
		case "memory":
			instance.Memory = leadingFloat(value)
		case "processor":
			instance.PhysicalProcessor = value
		case "hdd", "ssd":
			if value != "" {
				instance.LocalStorage = value
			}
		}
	}

	region := stringValue(object["Region"])
	if region != "" {
		instance.Regions = utils.AppendUnique(instance.Regions, region)
		zones := decodeAnonymousZones(stringValue(object["AvailableZone"]))
		for _, zone := range zones {
			instance.AvailabilityZones[region] = utils.AppendUnique(instance.AvailabilityZones[region], zone)
		}
		mergeAnonymousAvailability(instance, region, zones, anonymousPurchaseOption(object))
	}
}

func mergeAnonymousAvailability(instance *VEInstance, region string, zones []string, option string) {
	if len(zones) == 0 || option == "" {
		return
	}
	regionAvailability := instance.Availability[region]
	regionAvailability.Status = utils.AvailabilityAvailable
	regionAvailability.Evidence = utils.AvailabilityCatalog
	if regionAvailability.PurchaseOptions == nil {
		regionAvailability.PurchaseOptions = map[string]utils.AvailabilityStatus{}
	}
	if regionAvailability.Zones == nil {
		regionAvailability.Zones = map[string]utils.ZoneAvailability{}
	}
	regionAvailability.PurchaseOptions[option] = utils.AvailabilityAvailable
	for _, zone := range zones {
		zoneAvailability := regionAvailability.Zones[zone]
		zoneAvailability.Status = utils.AvailabilityAvailable
		if zoneAvailability.PurchaseOptions == nil {
			zoneAvailability.PurchaseOptions = map[string]utils.AvailabilityStatus{}
		}
		zoneAvailability.PurchaseOptions[option] = utils.AvailabilityAvailable
		regionAvailability.Zones[zone] = zoneAvailability
	}
	instance.Availability[region] = regionAvailability
}

func anonymousPurchaseOption(object map[string]any) string {
	switch {
	case stringValue(object["BillingMethod"]) == "configuration_hourly",
		stringValue(object["PayType"]) == "post":
		return "ondemand"
	case stringValue(object["BillingMethod"]) == "monthly",
		stringValue(object["PayType"]) == "pre":
		return "prepaid"
	default:
		return ""
	}
}

func mergeAnonymousPrice(all map[string]*VEInstance, object map[string]any) {
	if stringValue(object["Product"]) != "ECS" {
		return
	}
	instanceType := normalizeAnonymousInstanceType(stringValue(object["ConfigurationCode"]))
	instance := all[instanceType]
	if instance == nil {
		return
	}
	region := ""
	chargeItem := stringValue(object["ChargeItemCode"])
	for _, candidate := range instance.Regions {
		if strings.HasSuffix(chargeItem, "_"+candidate) {
			region = candidate
			break
		}
	}
	if region == "" {
		return
	}
	rawPrices, ok := object["PriceInfoList"].([]any)
	if !ok {
		return
	}
	prices := instance.Pricing[region]
	if prices == nil {
		prices = map[string]map[string]string{"linux": {}}
		instance.Pricing[region] = prices
	}
	for _, rawPrice := range rawPrices {
		price, ok := rawPrice.(map[string]any)
		if !ok {
			continue
		}
		key := anonymousPriceKey(stringValue(price["Period"]), intValue(price["Times"]))
		value := stringValue(price["Price"])
		if key != "" && value != "" {
			prices["linux"][key] = value
		}
	}
}

func normalizeAnonymousInstanceType(value string) string {
	value = strings.TrimSuffix(value, ".month")
	if !strings.HasPrefix(value, "ecs.") || strings.Count(value, ".") < 2 {
		return ""
	}
	return value
}

func decodeAnonymousZones(value string) []string {
	var zones []struct {
		ZoneCode string `json:"ZoneCode"`
	}
	if value == "" || json.Unmarshal([]byte(value), &zones) != nil {
		return nil
	}
	result := make([]string, 0, len(zones))
	for _, zone := range zones {
		if zone.ZoneCode != "" {
			result = append(result, zone.ZoneCode)
		}
	}
	return result
}

func anonymousPriceKey(period string, times int) string {
	switch {
	case period == "hourly":
		return "ondemand"
	case period == "monthly" && times == 1:
		return "monthly"
	case period == "monthly" && times >= 12 && times%12 == 0:
		return fmt.Sprintf("yearly_%d", times/12)
	default:
		return ""
	}
}

func stringValue(value any) string {
	switch current := value.(type) {
	case string:
		return current
	case json.Number:
		return current.String()
	case float64:
		return strconv.FormatFloat(current, 'f', -1, 64)
	default:
		return ""
	}
}

func intValue(value any) int {
	if number, ok := value.(float64); ok {
		return int(number)
	}
	result, _ := strconv.Atoi(stringValue(value))
	return result
}

func leadingInt(value string) int {
	var result int
	fmt.Sscanf(value, "%d", &result)
	return result
}

func leadingFloat(value string) float64 {
	var result float64
	fmt.Sscanf(value, "%f", &result)
	return result
}

// ----- optional API fetch -----

func fetchSpecsFromAPI(accessKey, secretKey string) map[string]*VEInstance {
	all := map[string]*VEInstance{}
	nextToken := ""
	for {
		types, next, err := describeInstanceTypes(accessKey, secretKey, nextToken)
		if err != nil {
			log.Printf("[volcengine] DescribeInstanceTypes error: %v", err)
			break
		}
		for _, t := range types {
			if t.InstanceTypeId == nil {
				continue
			}
			itype := *t.InstanceTypeId
			inst := &VEInstance{
				InstanceType:   itype,
				InstanceFamily: deref(t.InstanceTypeFamily),
				PrettyName:     prettyName(deref(t.InstanceTypeFamily)),
				Arch:           []string{"x86_64"},
				Pricing:        map[string]map[string]map[string]string{},
			}
			if t.Processor != nil && t.Processor.Cpus != nil {
				inst.VCPU = int(*t.Processor.Cpus)
			}
			if t.Memory != nil && t.Memory.Size != nil {
				inst.Memory = float64(*t.Memory.Size) / 1024.0
			}
			if t.GPU != nil && len(t.GPU.GPUDevices) > 0 {
				d := t.GPU.GPUDevices[0]
				if d.Count != nil {
					inst.GPU = int(*d.Count)
				}
				inst.GPUModel = deref(d.ProductName)
			}
			all[itype] = inst
		}
		if next == "" {
			break
		}
		nextToken = next
	}
	return all
}

func describeInstanceTypes(accessKey, secretKey, nextToken string) ([]veInstanceType, string, error) {
	query := url.Values{
		"Action":  {"DescribeInstanceTypes"},
		"Version": {"2020-04-01"},
	}
	if nextToken != "" {
		query.Set("NextToken", nextToken)
	}

	timestamp := time.Now().UTC()
	signedURL, headers, err := signVERequest(accessKey, secretKey, "GET", ecsAPIHost, "/", query.Encode(), "", timestamp)
	if err != nil {
		return nil, "", err
	}

	var out veDescribeResponse
	resp, err := utils.RetryingHTTPClient().R().
		SetHeaders(headers).
		SetSuccessResult(&out).
		Get(signedURL)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, signedURL)
	}
	if out.ResponseMetadata.Error != nil {
		return nil, "", fmt.Errorf("API %s: %s", out.ResponseMetadata.Error.Code, out.ResponseMetadata.Error.Message)
	}
	return out.Result.InstanceTypes, out.Result.NextToken, nil
}

// signVERequest implements the Volcengine HMAC-SHA256 signature scheme.
// Reference: https://www.volcengine.com/docs/6369/67269
func signVERequest(ak, sk, method, host, path, rawQuery, body string, t time.Time) (string, map[string]string, error) {
	date := t.Format("20060102")
	datetime := t.Format("20060102T150405Z")

	payloadHash := hashSHA256("")
	headers := map[string]string{
		"X-Date":           datetime,
		"Host":             host,
		"X-Content-Sha256": payloadHash,
	}

	// Canonical headers
	canonicalHeaders := fmt.Sprintf("host:%s\nx-content-sha256:%s\nx-date:%s\n", host, payloadHash, datetime)
	signedHeaders := "host;x-content-sha256;x-date"

	canonicalRequest := strings.Join([]string{
		method, path, rawQuery,
		canonicalHeaders, signedHeaders, payloadHash,
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/%s/request", date, ecsAPIRegion, ecsService)
	stringToSign := strings.Join([]string{
		"HMAC-SHA256", datetime, credentialScope, hashSHA256(canonicalRequest),
	}, "\n")

	signingKey := hmacSHA256Bytes(
		hmacSHA256Bytes(
			hmacSHA256Bytes(
				hmacSHA256Bytes([]byte(sk), date),
				ecsAPIRegion),
			ecsService),
		"request")
	signature := hex.EncodeToString(hmacSHA256Bytes(signingKey, stringToSign))

	headers["Authorization"] = fmt.Sprintf(
		"HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		ak, credentialScope, signedHeaders, signature,
	)

	fullURL := "https://" + host + path
	if rawQuery != "" {
		fullURL += "?" + rawQuery
	}
	return fullURL, headers, nil
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

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func prettyName(family string) string {
	f := strings.ToLower(family)
	switch {
	case strings.HasPrefix(f, "gn"):
		return "GPU " + strings.ToUpper(family)
	case strings.HasPrefix(f, "g"):
		return "General Purpose " + strings.ToUpper(family)
	case strings.HasPrefix(f, "c"):
		return "Compute Optimized " + strings.ToUpper(family)
	case strings.HasPrefix(f, "r"):
		return "Memory Optimized " + strings.ToUpper(family)
	case strings.HasPrefix(f, "i"):
		return "Storage Optimized " + strings.ToUpper(family)
	case strings.HasPrefix(f, "d"):
		return "Big Data " + strings.ToUpper(family)
	}
	return "Volcengine ECS " + strings.ToUpper(family)
}

// ----- main entry point -----

// DoVolcengineScraping is called from main.go.
func DoVolcengineScraping() error {
	log.Println("[volcengine] starting scrape")

	all, err := fetchAnonymousSpecs()
	if err != nil {
		log.Printf("[volcengine] anonymous pricing portal failed: %v — using static seed data", err)
		all = map[string]*VEInstance{}
		for _, s := range staticSeeds {
			all[s.InstanceType] = &VEInstance{
				InstanceType:   s.InstanceType,
				InstanceFamily: s.Family,
				PrettyName:     s.PrettyName,
				VCPU:           s.VCPU,
				Memory:         s.MemGiB,
				Arch:           s.Arch,
				GPU:            s.GPU,
				GPUModel:       s.GPUModel,
				Pricing:        map[string]map[string]map[string]string{},
				Regions:        []string{"cn-beijing", "cn-shanghai"},
			}
		}
	} else {
		log.Printf("[volcengine] anonymous pricing portal returned %d instances", len(all))
	}

	// Enrich from live API if credentials present.
	ak := os.Getenv("VOLCENGINE_ACCESS_KEY")
	sk := os.Getenv("VOLCENGINE_SECRET_KEY")
	if ak != "" && sk != "" {
		log.Println("[volcengine] credentials found, fetching live data …")
		apiInstances := fetchSpecsFromAPI(ak, sk)
		for k, v := range apiInstances {
			if existing, ok := all[k]; ok {
				v.Pricing = existing.Pricing
				v.AvailabilityZones = existing.AvailabilityZones
				v.Availability = existing.Availability
				v.PhysicalProcessor = existing.PhysicalProcessor
				v.LocalStorage = existing.LocalStorage
				for _, region := range existing.Regions {
					v.Regions = utils.AppendUnique(v.Regions, region)
				}
			}
			all[k] = v
		}
	} else {
		log.Println("[volcengine] no credentials — using static seed data only")
	}

	sorted := make([]*VEInstance, 0, len(all))
	for _, v := range all {
		sorted = append(sorted, v)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].InstanceType < sorted[j].InstanceType
	})

	if err := utils.SaveInstances(sorted, outputFilePath); err != nil {
		return fmt.Errorf("save Volcengine instances: %w", err)
	}
	log.Printf("[volcengine] wrote %d instances to %s", len(sorted), outputFilePath)
	return nil
}
