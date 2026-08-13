// Package alicloud scrapes Alibaba Cloud ECS instance type data.
//
// Data sources (no credentials required):
//
//  1. International pricing CDN:
//     https://g.alicdn.com/aliyun/ecs-price-info-intl/2.0.8/price/download/instancePrice.json
//     Contains on-demand USD pricing keyed as "region::instance_type::network::os::io".
//
//  2. China pricing CDN:
//     https://g.alicdn.com/aliyun/ecs-price-info/2.0.8/price/download/instancePrice.json
//     Same format, CNY pricing, China-region keys.
//
//  3. Instance specs are derived from the well-known naming scheme:
//     ecs.{family}.{size}  e.g. ecs.g6.xlarge  → 4 vCPU / 16 GiB
//     For families not covered by the pattern table, specs are fetched via the
//     public ECS OpenAPI (DescribeInstanceTypes) when ALICLOUD_ACCESS_KEY /
//     ALICLOUD_SECRET_KEY are present in the environment.
package alicloud

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/fuzzbuster/ec2instances-cli/utils"
	"log"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	intlPricingURL = "https://g.alicdn.com/aliyun/ecs-price-info-intl/2.0.8/price/download/instancePrice.json"
	cnPricingURL   = "https://g.alicdn.com/aliyun/ecs-price-info/2.0.8/price/download/instancePrice.json"
	outputFilePath = "alicloud/instances.json"
)

// ----- pricing response structs -----

type pricePeriod struct {
	Price  string `json:"price"`
	Period string `json:"period"`
}

type pricingEntry struct {
	Hours  []pricePeriod `json:"hours"`
	Months []pricePeriod `json:"months"`
	Years  []pricePeriod `json:"years"`
}

type pricingFile struct {
	Currency    string                  `json:"currency"`
	Version     string                  `json:"version"`
	PricingInfo map[string]pricingEntry `json:"pricingInfo"`
}

// ----- output instance struct -----

// AliInstance mirrors the EC2Instance JSON schema used by the frontend,
// with Alibaba-specific fields added.
type AliInstance struct {
	InstanceType string `json:"instance_type"`
	// InstanceFamily is the Alibaba "spec family" prefix, e.g. "g6", "c6", "r6".
	InstanceFamily string `json:"instance_family"`
	// PrettyName is a human-readable label derived from the family description table.
	PrettyName string `json:"pretty_name"`
	VCPU       int    `json:"vCPU"`
	// Memory in GiB.
	Memory float64 `json:"memory"`
	// Arch is a list of supported CPU architectures, e.g. ["x86_64"] or ["arm64"].
	Arch []string `json:"arch"`
	// NetworkPerformance is a descriptor string, e.g. "Up to 10 Gbps".
	NetworkPerformance string `json:"network_performance"`
	// GPU count (0 if not a GPU instance).
	GPU int `json:"GPU"`
	// Pricing maps region → os → {"ondemand": "0.123"}
	Pricing map[string]map[string]map[string]string `json:"pricing"`
	// Regions lists all regions where the instance type has been observed.
	Regions      []string           `json:"regions"`
	Availability utils.Availability `json:"availability,omitempty"`
}

// ----- instance naming heuristic -----

// sizeVCPU maps Alibaba ECS size suffixes to their vCPU count.
var sizeVCPU = map[string]int{
	"nano":     1,
	"micro":    1,
	"small":    1,
	"medium":   2,
	"large":    2,
	"xlarge":   4,
	"2xlarge":  8,
	"3xlarge":  12,
	"4xlarge":  16,
	"6xlarge":  24,
	"7xlarge":  28,
	"8xlarge":  32,
	"12xlarge": 48,
	"13xlarge": 52,
	"14xlarge": 56,
	"16xlarge": 64,
	"24xlarge": 96,
	"26xlarge": 104,
}

// familyRatios maps normalised family prefix to GiB-per-vCPU memory ratio.
var familyRatios = map[string]float64{
	"g":      4, // General purpose
	"u":      4,
	"c":      2, // Compute optimised
	"hfc":    2,
	"r":      8, // Memory optimised
	"re":     26,
	"hfr":    8,
	"hfg":    4, // High frequency general
	"d":      4, // Big data / storage
	"i":      4, // Local NVMe SSD
	"gn":     4, // GPU
	"vgn":    4,
	"ebm":    4, // Bare metal
	"sn1":    2,
	"sn2":    4,
	"se":     4,
	"t":      1, // Burstable
	"n":      4,
	"e":      4,
	"ic":     2, // Compute intensive (Intel)
	"ce":     2, // Compute entry-level
	"cm":     2, // Compute memory
	"ebmc":   2, // Bare metal compute
	"ebmg":   4, // Bare metal general
	"ebmhfg": 4, // Bare metal high freq general
	"mn":     4, // Shared
	"xn":     4, // Shared
	"sccg":   4, // SCC general
	"scch":   2, // SCC compute
	"f":      4, // FPGA
	"ga":     4, // General purpose AMD
}

var familyArchMap = map[string][]string{
	"g":      {"x86_64"},
	"c":      {"x86_64"},
	"r":      {"x86_64"},
	"gn":     {"x86_64"},
	"vgn":    {"x86_64"},
	"d":      {"x86_64"},
	"i":      {"x86_64"},
	"t":      {"x86_64"},
	"hfc":    {"x86_64"},
	"hfg":    {"x86_64"},
	"hfr":    {"x86_64"},
	"ic":     {"x86_64"},
	"ce":     {"x86_64"},
	"cm":     {"x86_64"},
	"ebmc":   {"x86_64"},
	"ebmg":   {"x86_64"},
	"ebmhfg": {"x86_64"},
	"mn":     {"x86_64"},
	"xn":     {"x86_64"},
	"sccg":   {"x86_64"},
	"scch":   {"x86_64"},
	"f":      {"x86_64"},
	"ga":     {"x86_64"},
}

var familyGPUMap = map[string]int{
	"gn":  1,
	"vgn": 1,
}

var familyPrettyNames = map[string]string{
	"g":      "General Purpose",
	"c":      "Compute Optimized",
	"r":      "Memory Optimized",
	"d":      "Big Data / Storage Optimized",
	"i":      "Local NVMe SSD",
	"gn":     "GPU / Heterogeneous",
	"vgn":    "GPU Virtual",
	"t":      "Burstable",
	"sn1":    "Shared Compute",
	"sn2":    "Shared General",
	"hfc":    "High Clock Speed Compute",
	"hfg":    "High Clock Speed General",
	"hfr":    "High Clock Speed Memory",
	"ebm":    "Bare Metal",
	"se":     "Super Large Memory",
	"re":     "High Ratio Memory",
	"n":      "Entry-Level",
	"e":      "Shared Entry-Level",
	"ic":     "Compute Intensive (Intel)",
	"ce":     "Compute Entry-Level",
	"cm":     "Compute Memory",
	"ebmc":   "Bare Metal Compute",
	"ebmg":   "Bare Metal General",
	"ebmhfg": "Bare Metal High Clock Speed General",
	"mn":     "Shared General",
	"xn":     "Shared General",
	"sccg":   "Super Computing Cluster General",
	"scch":   "Super Computing Cluster Compute",
	"f":      "FPGA Accelerated",
	"ga":     "General Purpose AMD",
}

// stripGenSuffix removes trailing digits and 2-char enhancement suffixes
// (ne / xe / se / ve) from a family string to get the base prefix.
// e.g. "g6ne" → "g", "hfc5" → "hfc", "sn1ne" → "sn1"
func stripGenSuffix(family string) string {
	s := family
	for _, sfx := range []string{"ne", "xe", "se", "ve", "is", "a", "i", "t", "e", "v", "s"} {
		if strings.HasSuffix(s, sfx) {
			s = s[:len(s)-len(sfx)]
			break
		}
	}
	if _, ok := familyRatios[s]; ok {
		return s
	}
	for len(s) > 0 && s[len(s)-1] >= '0' && s[len(s)-1] <= '9' {
		s = s[:len(s)-1]
	}
	return s
}

// normalizeFamily strips the -c encoding suffix and generation/enhancement suffixes,
// returning the base family for map lookups.
func normalizeFamily(family string) string {
	if dashIdx := strings.Index(family, "-c"); dashIdx >= 0 {
		family = family[:dashIdx]
	} else if dashIdx := strings.Index(family, "-lc"); dashIdx >= 0 {
		family = family[:dashIdx]
	}
	return stripGenSuffix(family)
}

// guessSpec attempts to parse vCPU and Memory (GiB) from the instance type name.
func guessSpec(instanceType string) (vcpu int, memGiB float64, ok bool) {
	name := strings.TrimPrefix(instanceType, "ecs.")
	dotIdx := strings.LastIndex(name, ".")
	if dotIdx < 0 {
		return
	}
	family := name[:dotIdx]
	size := name[dotIdx+1:]

	// handle explicit cpu/memory encoding, e.g. ecs.t5-c1m1.large, ecs.t5-lc1m1.small
	if dashIdx := strings.LastIndex(family, "-c"); dashIdx >= 0 {
		var cpu, mem int
		if n, _ := fmt.Sscanf(family[dashIdx+2:], "%dm%d", &cpu, &mem); n == 2 {
			if sv, ok2 := sizeVCPU[size]; ok2 {
				return cpu * sv, float64(mem * sv), true
			}
		}
		family = family[:dashIdx]
	} else if dashIdx := strings.LastIndex(family, "-lc"); dashIdx >= 0 {
		var cpu, mem int
		if n, _ := fmt.Sscanf(family[dashIdx+3:], "%dm%d", &cpu, &mem); n == 2 {
			if sv, ok2 := sizeVCPU[size]; ok2 {
				return cpu * sv, float64(mem * sv), true
			}
		}
		family = family[:dashIdx]
	}

	sv, ok2 := sizeVCPU[size]
	if !ok2 {
		return
	}

	base := stripGenSuffix(family)
	ratio, ratioOk := familyRatios[base]
	if !ratioOk {
		ratio = 4 // default: general purpose assumption
	}
	return sv, float64(sv) * ratio, true
}

func lookupArch(family string) []string {
	if a, ok := familyArchMap[family]; ok {
		return a
	}
	return []string{"x86_64"}
}

func lookupGPU(family string) int {
	return familyGPUMap[family]
}

func lookupPrettyName(family string) string {
	if p, ok := familyPrettyNames[family]; ok {
		return p
	}
	return "Alibaba Cloud ECS"
}

// ----- HTTP helper -----

func fetchJSON(url string, dest interface{}) error {
	body, err := utils.FetchWithRetry(url, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dest)
}

// ----- optional: signed ECS API for authoritative specs -----

type ecsAPIInstanceType struct {
	InstanceTypeID         string  `json:"InstanceTypeId"`
	CpuCoreCount           int     `json:"CpuCoreCount"`
	MemorySize             float64 `json:"MemorySize"` // GiB
	InstanceTypeFamily     string  `json:"InstanceTypeFamily"`
	GPUAmount              int     `json:"GPUAmount"`
	PhysicalProcessorModel string  `json:"PhysicalProcessorModel"`
}

type ecsDescribeResponse struct {
	InstanceTypes struct {
		InstanceType []ecsAPIInstanceType `json:"InstanceType"`
	} `json:"InstanceTypes"`
	NextToken string `json:"NextToken"`
}

// fetchSpecsFromAPI returns authoritative specs from ECS DescribeInstanceTypes
// when ALICLOUD_ACCESS_KEY / ALICLOUD_SECRET_KEY are present; nil otherwise.
func fetchSpecsFromAPI() map[string]*AliInstance {
	ak := os.Getenv("ALICLOUD_ACCESS_KEY")
	sk := os.Getenv("ALICLOUD_SECRET_KEY")
	if ak == "" || sk == "" {
		return nil
	}
	log.Println("[alicloud] credentials found, fetching specs from ECS API …")
	specs, err := describeInstanceTypes(ak, sk)
	if err != nil {
		log.Printf("[alicloud] DescribeInstanceTypes failed: %v — falling back to naming heuristic", err)
		return nil
	}
	log.Printf("[alicloud] fetched %d spec entries from ECS API", len(specs))
	return specs
}

// describeInstanceTypes calls the ECS RPC API without any SDK.
// Authentication uses HMAC-SHA1 as documented at:
// https://www.alibabacloud.com/help/en/sdk/developer-reference/rpc-mechanism
func describeInstanceTypes(ak, sk string) (map[string]*AliInstance, error) {
	const endpoint = "https://ecs.aliyuncs.com/"
	out := make(map[string]*AliInstance)
	nextToken := ""

	for {
		params := map[string]string{
			"Action":           "DescribeInstanceTypes",
			"Version":          "2014-05-26",
			"Format":           "JSON",
			"SignatureMethod":  "HMAC-SHA1",
			"SignatureVersion": "1.0",
			"SignatureNonce":   fmt.Sprintf("%d", time.Now().UnixNano()),
			"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
			"AccessKeyId":      ak,
		}
		if nextToken != "" {
			params["NextToken"] = nextToken
		}

		signedURL, err := signAliRequest(params, sk, endpoint)
		if err != nil {
			return nil, err
		}

		var resp ecsDescribeResponse
		if err := utils.LoadJson(signedURL, &resp); err != nil {
			return nil, err
		}

		for _, it := range resp.InstanceTypes.InstanceType {
			cleanFamily := normalizeFamily(it.InstanceTypeFamily)
			inst := &AliInstance{
				InstanceType:   it.InstanceTypeID,
				InstanceFamily: it.InstanceTypeFamily,
				PrettyName:     lookupPrettyName(cleanFamily),
				VCPU:           it.CpuCoreCount,
				Memory:         it.MemorySize,
				Arch:           lookupArch(cleanFamily),
				GPU:            it.GPUAmount,
			}
			out[it.InstanceTypeID] = inst
		}

		if resp.NextToken == "" {
			break
		}
		nextToken = resp.NextToken
	}
	return out, nil
}

// signAliRequest produces a signed GET URL using HMAC-SHA1 (RPC mechanism).
func signAliRequest(params map[string]string, secretKey, endpoint string) (string, error) {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, percentEncode(k)+"="+percentEncode(params[k]))
	}
	canonicalQS := strings.Join(parts, "&")
	stringToSign := "GET&" + percentEncode("/") + "&" + percentEncode(canonicalQS)

	sig := hmacSHA1B64(secretKey+"&", stringToSign)
	return endpoint + "?" + canonicalQS + "&Signature=" + percentEncode(sig), nil
}

func hmacSHA1B64(key, data string) string {
	mac := hmac.New(sha1.New, []byte(key))
	mac.Write([]byte(data))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func percentEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isUnreserved(c) {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func isUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~'
}

// ----- main entry point -----

// DoAlicloudScraping is called from main.go.
func DoAlicloudScraping() error {
	log.Println("[alicloud] starting scrape")

	var intlPricing, cnPricing pricingFile
	errIntl := fetchJSON(intlPricingURL, &intlPricing)
	if errIntl != nil {
		log.Printf("[alicloud] WARN: could not fetch intl pricing: %v", errIntl)
	}
	errCN := fetchJSON(cnPricingURL, &cnPricing)
	if errCN != nil {
		log.Printf("[alicloud] WARN: could not fetch CN pricing: %v", errCN)
	}
	if errIntl != nil && errCN != nil {
		return fmt.Errorf("both pricing sources failed: international: %v; China: %v", errIntl, errCN)
	}

	apiSpecs := fetchSpecsFromAPI()
	instances := buildInstances(intlPricing, cnPricing, apiSpecs)

	sorted := make([]*AliInstance, 0, len(instances))
	for _, v := range instances {
		sorted = append(sorted, v)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].InstanceType < sorted[j].InstanceType
	})

	if err := utils.SaveInstances(sorted, outputFilePath); err != nil {
		return fmt.Errorf("save Alicloud instances: %w", err)
	}
	log.Printf("[alicloud] wrote %d instances to %s", len(sorted), outputFilePath)
	return nil
}

func buildInstances(intl, cn pricingFile, apiSpecs map[string]*AliInstance) map[string]*AliInstance {
	out := map[string]*AliInstance{}

	process := func(pf pricingFile) {
		for key, entry := range pf.PricingInfo {
			// key: region::instance_type::network::os::io
			parts := strings.SplitN(key, "::", 5)
			if len(parts) < 2 {
				continue
			}
			region, itype := parts[0], parts[1]
			osLabel := "linux"
			if len(parts) >= 4 && parts[3] != "" {
				osLabel = parts[3]
			}

			inst, exists := out[itype]
			if !exists {
				inst = newInstance(itype, apiSpecs)
				out[itype] = inst
			}
			if inst.Pricing == nil {
				inst.Pricing = map[string]map[string]map[string]string{}
			}
			if inst.Pricing[region] == nil {
				inst.Pricing[region] = map[string]map[string]string{}
			}
			if inst.Pricing[region][osLabel] == nil {
				inst.Pricing[region][osLabel] = map[string]string{}
			}
			if len(entry.Hours) > 0 {
				inst.Pricing[region][osLabel]["ondemand"] = entry.Hours[0].Price
			}
			inst.Regions = utils.AppendUnique(inst.Regions, region)
			if len(entry.Hours) == 0 && len(entry.Months) == 0 && len(entry.Years) == 0 {
				continue
			}
			if inst.Availability == nil {
				inst.Availability = utils.Availability{}
			}
			regionAvailability := inst.Availability[region]
			regionAvailability.Status = utils.AvailabilityOffered
			regionAvailability.Evidence = utils.AvailabilityPricing
			if regionAvailability.PurchaseOptions == nil {
				regionAvailability.PurchaseOptions = map[string]utils.AvailabilityStatus{}
			}
			if len(entry.Hours) > 0 {
				regionAvailability.PurchaseOptions["ondemand"] = utils.AvailabilityOffered
			}
			if len(entry.Months) > 0 || len(entry.Years) > 0 {
				regionAvailability.PurchaseOptions["prepaid"] = utils.AvailabilityOffered
			}
			inst.Availability[region] = regionAvailability
		}
	}

	process(intl)
	process(cn)
	return out
}

func newInstance(itype string, apiSpecs map[string]*AliInstance) *AliInstance {
	if apiSpecs != nil {
		if spec, ok := apiSpecs[itype]; ok {
			return spec
		}
	}
	inst := &AliInstance{InstanceType: itype}
	name := strings.TrimPrefix(itype, "ecs.")
	if dotIdx := strings.Index(name, "."); dotIdx >= 0 {
		inst.InstanceFamily = name[:dotIdx]
	} else {
		inst.InstanceFamily = name
	}
	cleanFamily := normalizeFamily(inst.InstanceFamily)
	inst.PrettyName = lookupPrettyName(cleanFamily)
	inst.Arch = lookupArch(cleanFamily)
	inst.GPU = lookupGPU(cleanFamily)
	if vcpu, memGiB, ok := guessSpec(itype); ok {
		inst.VCPU = vcpu
		inst.Memory = memGiB
	}
	return inst
}
