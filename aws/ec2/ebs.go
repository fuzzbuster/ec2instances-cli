package ec2

import (
	"fmt"
	"github.com/fuzzbuster/ec2instances-cli/aws/awsutils"
	"log"
	"strconv"
)

type EBSConfig struct {
	Regions []EBSRegion `json:"regions"`
}

type EBSValueColumn struct {
	Prices map[string]string `json:"prices"`
}

type EBSStorageSize struct {
	Size         string           `json:"size"`
	ValueColumns []EBSValueColumn `json:"valueColumns"`
}

type EBSInstanceType struct {
	Sizes []EBSStorageSize `json:"sizes"`
}

type EBSRegion struct {
	Region        string            `json:"region"`
	InstanceTypes []EBSInstanceType `json:"instanceTypes"`
}

type EBSData struct {
	Config EBSConfig `json:"config"`
}

var EBS_REGION_MAP = map[string]string{
	"eu-ireland":   "eu-west-1",
	"eu-frankfurt": "eu-central-1",
	"apac-sin":     "ap-southeast-1",
	"apac-syd":     "ap-southeast-2",
	"apac-tokyo":   "ap-northeast-1",
}

func transformEbsRegionName(region string) (string, error) {
	if region, ok := EBS_REGION_MAP[region]; ok {
		return region, nil
	}

	// Parse region name to extract base and number
	// Pattern: ^([^0-9]*)(-(\d))?$
	// This matches a region name that optionally ends with a dash and number
	for i := len(region) - 1; i >= 0; i-- {
		if region[i] == '-' {
			// Check if what follows is a number
			if i+1 < len(region) {
				numStr := region[i+1:]
				if _, err := strconv.Atoi(numStr); err == nil {
					// Valid format: base-number
					return region, nil
				}
			}
			// Invalid format, treat as base-1
			return region + "-1", nil
		}
		if region[i] >= '0' && region[i] <= '9' {
			continue
		}
		// Found non-digit character, everything before this is the base
		// If no dash found, append -1
		return region + "-1", nil
	}

	return "", fmt.Errorf("cannot parse EBS region %q", region)
}

func addEBSPricing(instances map[string]*EC2Instance, currency string) error {
	log.Default().Println("Adding EBS pricing to EC2")

	var ebsData EBSData
	err := awsutils.FetchDataFromAWSWebsite(
		"https://a0.awsstatic.com/pricing/1/ec2/pricing-ebs-optimized-instances.min.js",
		&ebsData,
	)
	if err != nil {
		return fmt.Errorf("fetch EBS pricing data: %w", err)
	}

	for _, regionSpec := range ebsData.Config.Regions {
		region, err := transformEbsRegionName(regionSpec.Region)
		if err != nil {
			return err
		}
		for _, instanceTypeSpec := range regionSpec.InstanceTypes {
			for _, sizeSpec := range instanceTypeSpec.Sizes {
				instance := instances[sizeSpec.Size]
				if instance == nil {
					return fmt.Errorf("EBS pricing has unknown instance type %s", sizeSpec.Size)
				}
				pricingData := instance.Pricing[region]
				if pricingData == nil {
					pricingData = make(map[OS]any)
				}
				for _, col := range sizeSpec.ValueColumns {
					price, ok := col.Prices[currency]
					if !ok {
						return fmt.Errorf("EBS pricing has no %s price for %s", currency, sizeSpec.Size)
					}
					priceFloat, err := strconv.ParseFloat(price, 64)
					if err != nil {
						return fmt.Errorf("parse EBS price for %s: %w", sizeSpec.Size, err)
					}
					pricingData["ebs"] = formatPrice(priceFloat)
				}
			}
		}
	}
	return nil
}
