package aws

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync"

	"github.com/fuzzbuster/ec2instances-cli/aws/awsutils"
	ec2Internal "github.com/fuzzbuster/ec2instances-cli/aws/ec2"
	"github.com/fuzzbuster/ec2instances-cli/utils"
)

const (
	AWS_NON_CHINA_ROOT_URL = "https://pricing.us-east-1.amazonaws.com"
	AWS_CHINA_ROOT_URL     = "https://pricing.cn-north-1.amazonaws.com.cn"
)

func loadAwsURLJSON(baseURL, awsURL string, value any) error {
	if strings.HasPrefix(awsURL, "/") {
		awsURL = baseURL + awsURL
	}
	return utils.LoadJson(awsURL, value)
}

type awsRootIndexResponse struct {
	Offers map[string]struct {
		CurrentSavingsPlanIndexURL string `json:"currentSavingsPlanIndexUrl"`
		CurrentRegionIndexURL      string `json:"currentRegionIndexUrl"`
	} `json:"offers"`
}

type awsRegionIndexResponse struct {
	Regions map[string]struct {
		CurrentVersionURL string `json:"currentVersionUrl"`
	} `json:"regions"`
}

type flatRegionData struct {
	name string
	url  string
}

func loadEC2Regions(
	baseURL string,
	root awsRootIndexResponse,
	china bool,
	output chan<- awsutils.RawRegion,
) (err error) {
	defer close(output)

	offer, ok := root.Offers["AmazonEC2"]
	if !ok {
		return fmt.Errorf("AmazonEC2 offer not found")
	}

	savingsPlans := func() (map[string]map[string]map[string]float64, error) {
		return map[string]map[string]map[string]float64{}, nil
	}
	if offer.CurrentSavingsPlanIndexURL != "" {
		savingsPlans = awsutils.GetSavingsPlans(baseURL, offer.CurrentSavingsPlanIndexURL, china)
	}

	var regionIndex awsRegionIndexResponse
	if err := loadAwsURLJSON(baseURL, offer.CurrentRegionIndexURL, &regionIndex); err != nil {
		return fmt.Errorf("load EC2 region index: %w", err)
	}

	regions := make([]flatRegionData, 0, len(regionIndex.Regions))
	for name, metadata := range regionIndex.Regions {
		if (!china && name == "cn-north-1-pkx-1") || (china && name == "aws-cn-other") {
			continue
		}
		regions = append(regions, flatRegionData{name: name, url: metadata.CurrentVersionURL})
	}

	for _, chunk := range utils.Chunk(regions, 5) {
		var group utils.FunctionGroup
		for _, region := range chunk {
			group.Add(func() error {
				var data awsutils.RegionData
				if err := loadAwsURLJSON(baseURL, region.url, &data); err != nil {
					return fmt.Errorf("load EC2 pricing for %s: %w", region.name, err)
				}
				output <- awsutils.RawRegion{RegionName: region.name, RegionData: data}
				return nil
			})
		}
		if err := group.Run(); err != nil {
			return err
		}
		runtime.GC()
	}

	output <- awsutils.RawRegion{SavingsPlanData: savingsPlans}
	return nil
}

var REGIONS_ITERATOR = []string{
	"us-east-1",
	"us-east-2",
	"us-west-1",
	"us-west-2",
	"eu-west-1",
	"eu-west-2",
	"eu-central-1",
}

func crossRegionDescribeInstanceTypesIterator(pushChunk func(map[string]*ec2Internal.APIInstanceTypeInfo)) error {
	seen := map[string]struct{}{}
	var seenMu sync.Mutex
	client := newEC2APIClient()

	var group utils.FunctionGroup
	for _, region := range REGIONS_ITERATOR {
		group.Add(func() error {
			pages, err := client.describeInstanceTypes(region, func(output []ec2Internal.APIInstanceTypeInfo) {
				instanceTypes := make(map[string]*ec2Internal.APIInstanceTypeInfo)
				seenMu.Lock()
				for i := range output {
					instanceType := output[i].InstanceType
					if _, ok := seen[instanceType]; ok {
						continue
					}
					seen[instanceType] = struct{}{}
					instanceTypes[instanceType] = &output[i]
				}
				seenMu.Unlock()
				if len(instanceTypes) > 0 {
					pushChunk(instanceTypes)
					log.Printf("Processed %d unique instance type descriptions for %s", len(instanceTypes), region)
				}
				runtime.GC()
			})
			if err != nil {
				if pages == 0 && !isEC2RateLimitError(err) && region != "us-east-1" {
					return nil
				}
				return fmt.Errorf("describe EC2 instance types in %s: %w", region, err)
			}
			return nil
		})
	}
	return group.Run()
}

// DoAwsScraping scrapes EC2 virtual-machine data and pricing.
func DoAwsScraping() error {
	return scrapeAnonymousEC2()
}
