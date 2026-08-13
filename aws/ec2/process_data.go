package ec2

import (
	"fmt"
	"github.com/fuzzbuster/ec2instances-cli/aws/awsutils"
	"github.com/fuzzbuster/ec2instances-cli/utils"
	"sort"
	"strconv"
	"strings"

	"github.com/anaskhan96/soup"
)

type ec2DataGetters struct {
	t2HtmlGetter func() (*soup.Root, error)
}

var EC2_OK_PRODUCT_FAMILIES = map[string]bool{
	"Compute Instance":              true,
	"Compute Instance (bare metal)": true,
	"Dedicated Host":                true,
}

var EC2_ADD_METAL = map[string]bool{
	"u-6tb1":  true,
	"u-9tb1":  true,
	"u-12tb1": true,
}

type ec2SkuData struct {
	instance      *EC2Instance
	platform      string
	hasBoxUsage   bool
	region        string
	onDemandPrice float64
}

func processEC2Data(
	inData chan awsutils.RawRegion,
	ec2ApiResponses *utils.SlowBuildingMap[string, *APIInstanceTypeInfo],
	china bool,
	getters ec2DataGetters,
) error {
	// Defines the currency
	currency := "USD"
	if china {
		currency = "CNY"
	}

	// Data that is used throughout the process
	instancesHashmap := make(map[string]*EC2Instance)
	sku2SkuData := make(map[string]ec2SkuData)

	// The descriptions found for each region
	regionDescriptions := make(map[string]string)

	// Process each region as it comes in
	var savingsPlanData func() (map[string]map[string]map[string]float64, error)
	for rawRegion := range inData {
		// Close the channel when we're done
		if rawRegion.SavingsPlanData != nil {
			savingsPlanData = rawRegion.SavingsPlanData
			close(inData)
			break
		}

		// Process the products in the region
		regionDescription := ""
		for _, product := range rawRegion.RegionData.Products {
			if _, ok := EC2_OK_PRODUCT_FAMILIES[product.ProductFamily]; !ok {
				continue
			}

			instanceType := product.Attributes["instanceType"]
			if instanceType == "" {
				continue
			}

			location := product.Attributes["location"]
			if location != "" {
				if regionDescription != "" && regionDescription != location {
					return fmt.Errorf("EC2 region description mismatch: %q and %q for %s", regionDescription, location, instanceType)
				}
				regionDescription = location
			}

			if _, ok := EC2_ADD_METAL[instanceType]; ok {
				instanceType = instanceType + ".metal"
			}

			pieces := strings.Split(instanceType, ".")
			if len(pieces) == 1 {
				// Dedicated host that is not u-*.metal, skipping
				// May be a good idea to all dedicated hosts in the future
				continue
			}

			instance := instancesHashmap[instanceType]
			if instance == nil {
				instance = &EC2Instance{
					InstanceType:             instanceType,
					Pricing:                  make(map[Region]map[OS]any),
					LinuxVirtualizationTypes: []string{},
					VpcOnly:                  true,
					PlacementGroupSupport:    true,
					IPV6Support:              true,

					// TODO: Figure out why this is always empty in Python code, and
					// make a fixed version for here
					AvailabilityZones: make(map[string][]string),
				}
				instance.addExtraDetails()
				instancesHashmap[instanceType] = instance
			}

			platform := awsutils.TranslatePlatformName(
				product.Attributes["operatingSystem"],
				product.Attributes["preInstalledSw"],
			)
			if platform != "" && shouldIncludeEC2PricingSku(platform, product.Attributes["licenseModel"]) {
				sku2SkuData[product.SKU] = ec2SkuData{
					instance:    instance,
					platform:    platform,
					hasBoxUsage: strings.Contains(product.Attributes["usagetype"], "BoxUsage"),
					region:      rawRegion.RegionName,
				}
			}

			if err := enrichEc2Instance(instance, product.Attributes, ec2ApiResponses); err != nil {
				return err
			}
		}

		// Gets the pricing data for the region/platform. Creates if it doesn't exist.
		getPricingData := func(instance *EC2Instance, platform string) *EC2PricingData {
			regionMap := instance.Pricing[rawRegion.RegionName]
			if regionMap == nil {
				regionMap = make(map[OS]any)
				instance.Pricing[rawRegion.RegionName] = regionMap
			}
			osMap := regionMap[platform]
			if osMap == nil {
				m := make(map[string]string)
				osMap = &EC2PricingData{
					Reserved: &m,
					OnDemand: "0",
				}
				regionMap[platform] = osMap
			}
			return osMap.(*EC2PricingData)
		}

		// Process the on demand pricing
		for _, offerMapping := range rawRegion.RegionData.Terms.OnDemand {
			for _, offer := range offerMapping {
				// Get the instance in question
				skuData, ok := sku2SkuData[offer.SKU]
				if !ok {
					continue
				}
				instance := skuData.instance
				platform := skuData.platform

				if !skuData.hasBoxUsage {
					// This is a magic thing that breaks pricing. Ignore anything with it.
					continue
				}

				// Get the price dimension
				if len(offer.PriceDimensions) != 1 {
					return fmt.Errorf("EC2 on-demand offer %s for %s has %d price dimensions", offer.SKU, instance.InstanceType, len(offer.PriceDimensions))
				}
				var priceDimension awsutils.RegionPriceDimension
				for _, priceDimension = range offer.PriceDimensions {
					// Intentionally empty - this just gets the first one
				}

				// Get the price
				if priceDimension.PricePerUnit != nil {
					usd, ok := priceDimension.PricePerUnit[currency]
					if ok {
						usdFloat, err := strconv.ParseFloat(usd, 64)
						if err != nil {
							return fmt.Errorf("parse EC2 price for offer %s instance %s: %w", offer.SKU, instance.InstanceType, err)
						}
						pricingData := getPricingData(instance, platform)
						if usdFloat == 0 {
							// No such thing as a free lunch
							continue
						}
						old, err := strconv.ParseFloat(pricingData.OnDemand, 64)
						if err != nil && pricingData.OnDemand != "" {
							return fmt.Errorf("parse existing EC2 price %q for offer %s instance %s: %w", pricingData.OnDemand, offer.SKU, instance.InstanceType, err)
						}
						if old < usdFloat {
							pricingData.OnDemand = formatPrice(usdFloat)
						}
						skuData.onDemandPrice = usdFloat
						sku2SkuData[offer.SKU] = skuData
					}
				}
			}
		}

		// Process the reserved pricing
		for _, offerMapping := range rawRegion.RegionData.Terms.Reserved {
			for _, offer := range offerMapping {
				// Get the instance in question
				skuData, ok := sku2SkuData[offer.SKU]
				if !ok {
					continue
				}

				if !skuData.hasBoxUsage {
					// This is a magic thing that breaks pricing. Ignore anything with it.
					continue
				}

				pricingData := getPricingData(skuData.instance, skuData.platform)
				if !skuOnDemandMatchesPlatform(skuData.onDemandPrice, pricingData.OnDemand) {
					continue
				}

				// Process this reserved offer
				if err := processReservedOffer(
					pricingData,
					offer.PriceDimensions,
					offer.TermAttributes,
					currency,
				); err != nil {
					return err
				}
			}
		}

		// Set the region description
		if regionDescription == "" {
			return fmt.Errorf("EC2 region description missing for %s", rawRegion.RegionName)
		} else {
			regionDescriptions[rawRegion.RegionName] = regionDescription
		}
	}
	if savingsPlanData == nil {
		return fmt.Errorf("EC2 pricing stream ended before savings plan data")
	}

	// Add EBS pricing if not China
	if !china {
		if err := addEBSPricing(instancesHashmap, currency); err != nil {
			return err
		}
	}

	// Add T2 credits
	if err := addT2Credits(instancesHashmap, getters.t2HtmlGetter); err != nil {
		return err
	}

	// Invert the regions map
	regionsInverted := make(map[string]string)
	for region, regionName := range regionDescriptions {
		regionsInverted[regionName] = region
	}

	// Some hacks to make some AWS API expectations work
	if !china {
		regionsInverted["AWS GovCloud (US)"] = "us-gov-west-1"
		regionsInverted["EU (Spain)"] = "eu-south-2"
		regionsInverted["EU (Zurich)"] = "eu-central-2"
	}

	// Add GPU information
	addGpuInfo(instancesHashmap)

	// Add FPGA information for instances AWS does not report via the API
	addFpgaInfo(instancesHashmap)

	// Add instance store random read/write IOPS from the AWS instance-type docs
	if err := addStorageIopsInfo(instancesHashmap); err != nil {
		return err
	}

	// Add placement group information
	addPlacementGroupInfo(instancesHashmap)

	// Add dedicated host pricing
	if china {
		if err := addDedicatedHostPricingCn(instancesHashmap, regionsInverted); err != nil {
			return err
		}
	} else {
		if err := addDedicatedHostPricingUs(instancesHashmap, regionsInverted); err != nil {
			return err
		}
	}

	// Add Linux AMI info
	addLinuxAmiInfo(instancesHashmap)

	// Add VPC only instances
	addVpcOnlyInstances(instancesHashmap)

	// Add date introduced from instancetyp.es timeline
	addDateIntroduced(instancesHashmap)

	// Add savings plans pricing
	savingsPlans, err := savingsPlanData()
	if err != nil {
		return fmt.Errorf("load EC2 savings plans: %w", err)
	}
	for region, skuMap := range savingsPlans {
		for sku, termMap := range skuMap {
			skuInfo, ok := sku2SkuData[sku]
			if !ok {
				continue
			}
			for term, price := range termMap {
				regionPricing, ok := skuInfo.instance.Pricing[region]
				if !ok {
					regionPricing = make(map[OS]any)
					skuInfo.instance.Pricing[region] = regionPricing
				}
				osPricing, ok := regionPricing[skuInfo.platform]
				if !ok {
					osPricing = &EC2PricingData{
						Reserved: &map[string]string{},
						OnDemand: "0",
					}
					regionPricing[skuInfo.platform] = osPricing
				}
				pricingData := osPricing.(*EC2PricingData)
				if !skuOnDemandMatchesPlatform(skuInfo.onDemandPrice, pricingData.OnDemand) {
					continue
				}
				if pricingData.Reserved == nil {
					m := make(map[string]string)
					pricingData.Reserved = &m
				}
				(*pricingData.Reserved)[term] = formatPrice(price)
			}
		}
	}

	// Clean up empty regions and set the regions map for non-empty regions
	for _, instance := range instancesHashmap {
		instance.Regions = cleanEmptyRegions(instance.Pricing, regionDescriptions)
	}

	// Save the instances
	sortedInstances := make([]*EC2Instance, 0, len(instancesHashmap))
	for _, instance := range instancesHashmap {
		sortedInstances = append(sortedInstances, instance)
	}
	sort.Slice(sortedInstances, func(i, j int) bool {
		return sortedInstances[i].InstanceType < sortedInstances[j].InstanceType
	})
	fp := "instances.json"
	if china {
		fp = "instances-cn.json"
	}
	if err := utils.SaveInstances(sortedInstances, fp); err != nil {
		return fmt.Errorf("save EC2 instances: %w", err)
	}
	return nil
}
