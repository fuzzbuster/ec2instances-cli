package aws

import (
	"fmt"
	"log"
	"os"
	"sync"

	ec2Internal "github.com/fuzzbuster/ec2instances-cli/aws/ec2"
	"github.com/fuzzbuster/ec2instances-cli/utils"
)

const compactEC2PricingURL = "https://b0.p.awsstatic.com/pricing/2.0/meteredUnitMaps/ec2/USD/current/ec2.json"

var ec2SpecificationURLs = []string{
	"https://docs.aws.amazon.com/ec2/latest/instancetypes/gp.md",
	"https://docs.aws.amazon.com/ec2/latest/instancetypes/co.md",
	"https://docs.aws.amazon.com/ec2/latest/instancetypes/mo.md",
	"https://docs.aws.amazon.com/ec2/latest/instancetypes/so.md",
	"https://docs.aws.amazon.com/ec2/latest/instancetypes/ac.md",
	"https://docs.aws.amazon.com/ec2/latest/instancetypes/hpc.md",
	"https://docs.aws.amazon.com/ec2/latest/instancetypes/pg.md",
}

func scrapeAnonymousEC2() error {
	documents := make([][]byte, len(ec2SpecificationURLs))
	for i, url := range ec2SpecificationURLs {
		body, err := utils.FetchWithRetry(url, nil)
		if err != nil {
			return fmt.Errorf("load EC2 specification %s: %w", url, err)
		}
		documents[i] = body
	}

	pricing, err := utils.FetchWithRetry(compactEC2PricingURL, nil)
	if err != nil {
		return fmt.Errorf("load compact EC2 pricing: %w", err)
	}
	return ec2Internal.ProcessAnonymousData(documents, pricing, loadOptionalEC2Descriptions())
}

func loadOptionalEC2Descriptions() map[string]*ec2Internal.APIInstanceTypeInfo {
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" || os.Getenv("AWS_SECRET_ACCESS_KEY") == "" {
		return nil
	}

	descriptions := map[string]*ec2Internal.APIInstanceTypeInfo{}
	var mu sync.Mutex
	err := crossRegionDescribeInstanceTypesIterator(func(chunk map[string]*ec2Internal.APIInstanceTypeInfo) {
		mu.Lock()
		defer mu.Unlock()
		for instanceType, description := range chunk {
			descriptions[instanceType] = description
		}
	})
	if err != nil {
		log.Printf("[aws] WARN DescribeInstanceTypes supplement failed: %v", err)
		return nil
	}
	return descriptions
}
