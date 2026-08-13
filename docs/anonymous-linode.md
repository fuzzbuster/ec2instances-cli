# Linode without credentials

Linode exposes instance types and regions through unauthenticated official REST
endpoints.

## Data sources

- Types: `https://api.linode.com/v4/linode/types?page_size=500`
- Regions: `https://api.linode.com/v4/regions?page_size=500`

The scraper follows numeric pages, joins regional price overrides and emits
vCPU, memory, SSD size, transfer, network and GPU or accelerated-device fields.
Monthly prices are divided by 730 only when the corresponding hourly price is
missing.

## Availability rules

No token is read or required. Regions must be active and advertise the
`Linodes` capability. GPU, Premium and NETINT accelerated types are restricted
to regions that advertise their respective capability.

Matching regions produce `availability` with `status: "available"`,
`evidence: "catalog"` and the `ondemand` purchase option. Region health and
capabilities describe catalog support; they do not report per-plan realtime
capacity.

## Failure behavior

Request, pagination or JSON decoding failures fail the scrape. There is no
static fallback because the public API is the authoritative catalog.
