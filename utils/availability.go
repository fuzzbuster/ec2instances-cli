package utils

type AvailabilityStatus string

const (
	AvailabilityAvailable AvailabilityStatus = "available"
	AvailabilityLimited   AvailabilityStatus = "limited"
	AvailabilitySoldOut   AvailabilityStatus = "sold_out"
	AvailabilityOffered   AvailabilityStatus = "offered"
	AvailabilityUnknown   AvailabilityStatus = "unknown"
)

type AvailabilityEvidence string

const (
	AvailabilityRealtime AvailabilityEvidence = "realtime"
	AvailabilityCatalog  AvailabilityEvidence = "catalog"
	AvailabilityPricing  AvailabilityEvidence = "pricing"
)

type Availability map[string]RegionAvailability

type RegionAvailability struct {
	Status          AvailabilityStatus            `json:"status"`
	Evidence        AvailabilityEvidence          `json:"evidence"`
	PurchaseOptions map[string]AvailabilityStatus `json:"purchase_options,omitempty"`
	Zones           map[string]ZoneAvailability   `json:"zones,omitempty"`
}

type ZoneAvailability struct {
	Status          AvailabilityStatus            `json:"status"`
	PurchaseOptions map[string]AvailabilityStatus `json:"purchase_options,omitempty"`
}
