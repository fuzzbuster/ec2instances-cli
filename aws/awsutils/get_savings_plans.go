package awsutils

import (
	"fmt"
	"github.com/fuzzbuster/ec2instances-cli/utils"
	"strings"
	"sync"
)

type (
	regionSlug = string
	sku        = string
	term       = string
)

func loadAwsUrlJson(baseUrl string, awsUrl string, val any) error {
	if strings.HasPrefix(awsUrl, "/") {
		awsUrl = baseUrl + awsUrl
	}
	return utils.LoadJson(awsUrl, val)
}

// savingsPlanTermSuffix maps the AWS productFamily field to the middle segment.
func savingsPlanTermSuffix(productFamily string) string {
	switch productFamily {
	case "EC2InstanceSavingsPlans":
		return "InstanceSavings"
	default:
		return "Savings" // Compute, SageMaker, etc.
	}
}

func translateReservedTermAttributes(purchaseTerm, productFamily, purchaseOption string) (string, error) {
	lease := LEASES[purchaseTerm]
	option := PURCHASE_OPTIONS[purchaseOption]

	if lease == "" || option == "" || productFamily == "" {
		return "", fmt.Errorf("EC2 savings plan has unknown term: %s %s %s", purchaseTerm, productFamily, purchaseOption)
	}

	return lease + savingsPlanTermSuffix(productFamily) + "." + option, nil
}

func processSavingsPlanRegion(
	rawRegion RawSavingsPlanRegion,
	isChina bool,
	write func(sku, term, float64),
) error {
	sku2product := make(map[string]map[string]string)
	for _, product := range rawRegion.Products {
		sku2product[product.SKU] = product.Attributes
	}

	for _, t := range rawRegion.Terms.SavingsPlan {
		productAttributes, ok := sku2product[t.SKU]
		if !ok {
			return fmt.Errorf("product not found for savings plan SKU %s", t.SKU)
		}

		purchaseOption := productAttributes["purchaseOption"]
		productFamily := productAttributes["productFamily"]
		purchaseTerm := productAttributes["purchaseTerm"]

		termKey, err := translateReservedTermAttributes(purchaseTerm, productFamily, purchaseOption)
		if err != nil {
			return err
		}

		for _, rate := range t.Rates {
			price, err := Floaty(rate.DiscountedRate.Price)
			if err != nil {
				return err
			}
			currency := "USD"
			if isChina {
				currency = "CNY"
			}
			if rate.DiscountedRate.Currency != currency {
				return fmt.Errorf("savings plan currency mismatch for SKU %s: expected %s, got %s", t.SKU, currency, rate.DiscountedRate.Currency)
			}

			write(sku(rate.DiscountedSKU), term(termKey), price)
		}
	}
	return nil
}

type awsSavingsPlanRegion struct {
	RegionCode string `json:"regionCode"`
	VersionUrl string `json:"versionUrl"`
}

type awsSavingsPlansIndexResponse struct {
	Regions []awsSavingsPlanRegion `json:"regions"`
}

func processSavingsPlans(
	baseUrl, currentSavingsPlanIndexUrl string,
	isChina bool,
	write func(regionSlug, sku, term, float64),
) error {
	var savingsPlansData awsSavingsPlansIndexResponse
	err := loadAwsUrlJson(baseUrl, currentSavingsPlanIndexUrl, &savingsPlansData)
	if err != nil {
		return err
	}

	var fg utils.FunctionGroup
	for _, regionMeta := range savingsPlansData.Regions {
		fg.Add(func() error {
			var rawRegion RawSavingsPlanRegion
			err := loadAwsUrlJson(baseUrl, regionMeta.VersionUrl, &rawRegion)
			if err != nil {
				return err
			}

			regionWrite := func(s sku, t term, price float64) {
				write(regionSlug(regionMeta.RegionCode), s, t, price)
			}
			return processSavingsPlanRegion(rawRegion, isChina, regionWrite)
		})
	}
	return fg.Run()
}

// GetSavingsPlans is used to get the savings plans data for a specific servica/base URL
// Returns a function to get the map of regions to their instances and their savings plans data
func GetSavingsPlans(baseUrl, currentSavingsPlanIndexUrl string, isChina bool) func() (map[regionSlug]map[sku]map[term]float64, error) {
	m := make(map[regionSlug]map[sku]map[term]float64)
	mu := sync.Mutex{}
	write := func(region regionSlug, s sku, t term, price float64) {
		mu.Lock()
		defer mu.Unlock()
		regionMap, ok := m[region]
		if !ok {
			regionMap = make(map[sku]map[term]float64)
			m[region] = regionMap
		}
		instanceMap, ok := regionMap[s]
		if !ok {
			instanceMap = make(map[term]float64)
			regionMap[s] = instanceMap
		}
		instanceMap[t] = price
	}

	return utils.BlockUntilDone(func() (map[regionSlug]map[sku]map[term]float64, error) {
		if err := processSavingsPlans(baseUrl, currentSavingsPlanIndexUrl, isChina, write); err != nil {
			return nil, err
		}
		return m, nil
	})
}
