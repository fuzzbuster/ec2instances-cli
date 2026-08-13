package ec2

import (
	"fmt"
	"github.com/fuzzbuster/ec2instances-cli/aws/awsutils"
	"regexp"
	"strconv"
)

var START_NUMBERS = regexp.MustCompile(`^(\d+)`)

func processReservedOffer(
	pricingData *EC2PricingData,
	priceDimensions map[string]awsutils.RegionPriceDimension,
	termAttributes map[string]string,
	currency string,
) error {
	// Go through the price dimensions to get the upfront and hourly prices
	upfrontPrice := 0.0
	pricePerHour := 0.0
	for _, priceDimension := range priceDimensions {
		tempPrice := 0.0
		if priceDimension.PricePerUnit != nil {
			usd, ok := priceDimension.PricePerUnit[currency]
			if ok {
				usdFloat, err := strconv.ParseFloat(usd, 64)
				if err != nil {
					return fmt.Errorf("parse EC2 reserved price %q: %w", usd, err)
				}
				tempPrice = usdFloat
			}
		}

		if priceDimension.Unit == "Hrs" {
			pricePerHour = tempPrice
		} else {
			upfrontPrice = tempPrice
		}
	}

	// Translate the term attributes into a term code
	localTerm, err := translateReservedTermAttributes(termAttributes)
	if err != nil {
		return err
	}

	// Get the price per hour
	startNumber := START_NUMBERS.FindString(termAttributes["LeaseContractLength"])
	if startNumber == "" {
		return fmt.Errorf("EC2 reserved pricing term %s has no lease length", localTerm)
	}
	leaseInYears, err := strconv.Atoi(startNumber)
	if err != nil {
		return fmt.Errorf("parse EC2 reserved lease length for %s: %w", localTerm, err)
	}
	hoursInTerm := leaseInYears * 365 * 24
	finalPrice := pricePerHour + (upfrontPrice / float64(hoursInTerm))

	// Write to the pricing data
	(*pricingData.Reserved)[localTerm] = formatPrice(finalPrice)
	return nil
}
