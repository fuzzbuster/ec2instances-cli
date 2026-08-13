# Google Compute Engine credential requirements

Google Compute Engine is the exception among the supported providers: the
current scraper cannot collect its dataset anonymously.

## Why credentials are required

The scraper uses the Cloud Billing Catalog API for SKUs and prices and the
Compute Engine API for machine types and regions:

- `https://cloudbilling.googleapis.com`
- `https://compute.googleapis.com/compute/v1`

Anonymous requests to the required endpoints return authorization failures
(observed as HTTP 403). Public product pages do not expose an equivalent,
structured regional specification and pricing dataset.

Because there is no anonymous dataset, GCP does not participate in the
normalized anonymous `availability` output described in
[instance availability](availability.md).

## Required configuration

- `GCP_PROJECT_ID`
- `GCP_CLIENT_EMAIL`
- `GCP_PRIVATE_KEY`

The service account requests `compute.readonly` and `cloud-billing.readonly`
OAuth scopes. See [GCP setup](setting-up-gcp.md).

## Failure behavior

The CLI validates the required variables before starting a GCP scrape. Missing
credentials are a configuration error; there is no anonymous or static
fallback.
