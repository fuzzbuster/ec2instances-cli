# Azure Virtual Machines without credentials

Azure base specifications and regional prices do not require credentials.

## Data sources

- Public metadata:
  `https://azure.microsoft.com/api/v4/pricing/virtual-machines/metadata/`.
- Public VM offer and detail endpoints under
  `https://azure.microsoft.com/api/v3/pricing/virtual-machines/`.

The metadata response supplies regions and operating systems. The public offer
responses provide VM names, cores, memory, disk size and regional pricing.
Offer keys are split at the first and last dash so constrained-vCPU names such
as `e16-4as-v4` remain intact.

## Availability

Published regional prices produce `availability` with `status: "offered"` and
`evidence: "pricing"`. Purchase options are derived from the available price
keys and may include `ondemand`, `spot`, `lowpriority` and `reserved`. Public
pricing does not prove subscription eligibility, deployment success or current
capacity.

## Optional credentials

When all four variables are set, the scraper also reads Compute SKU
capabilities from Azure Resource Manager:

- `AZURE_TENANT_ID`
- `AZURE_CLIENT_ID`
- `AZURE_CLIENT_SECRET`
- `AZURE_SUBSCRIPTION_ID`

Partial credentials are ignored. See [Azure setup](setting-up-azure.md).

## Limits and failure behavior

Without credentials, Resource Manager capability fields may be absent, but base
specifications and prices remain available. Public endpoint or parsing failures
fail the scrape; there is no static fallback.
