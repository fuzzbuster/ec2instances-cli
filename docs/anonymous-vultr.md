# Vultr without credentials

Vultr exposes its compute catalog through unauthenticated official REST
endpoints.

## Data sources

- Plans: `https://api.vultr.com/v2/plans?per_page=500`
- Regions: `https://api.vultr.com/v2/regions?per_page=500`

Both endpoints return JSON. The scraper follows the opaque cursor in
`meta.links.next`, joins plan location IDs to region names and emits vCPU,
memory, disk, transfer, GPU metadata and per-region hourly pricing. A monthly
price is divided by 730 only when an hourly price is absent.

## Availability

Each plan location produces `availability` with `status: "available"` and
`evidence: "catalog"`. The public `deploy_ondemand` and
`deploy_preemptible` flags map to the `ondemand` and `preemptible` purchase
options. This is public deployment catalog data, not a realtime capacity
guarantee.

## Credentials and filtering

No Vultr API key is read or required. Plans without locations are excluded
because they cannot produce valid regional availability or pricing records.
Location-specific prices override the plan default.

## Failure behavior

Request, JSON decoding or pagination failures fail the scrape. There is no
static fallback because the public API is the authoritative catalog.
