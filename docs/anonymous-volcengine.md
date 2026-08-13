# Volcengine ECS without credentials

Volcengine ECS scraping uses the public pricing portal without access keys.

## Data sources

The scraper calls `https://www.volcengine.com/anonymous-api/trade/price` with
the ECS calculator template code `CLT7310571628986405164`:

1. `ListTemplate` returns the nested ECS specification template.
2. `GetTable` returns the associated price table.

The template is walked recursively to collect instance type, vCPU, memory,
processor, local storage, region and availability zones. The price table adds
hourly, monthly and yearly Linux prices.

## Availability

Template regions and `AvailableZone` values produce `availability` with
`status: "available"` and `evidence: "catalog"`. Hourly/postpaid templates map
to `ondemand`; monthly/prepaid templates map to `prepaid`. These values describe
calculator catalog coverage and do not guarantee realtime capacity.

## Optional credentials

`VOLCENGINE_ACCESS_KEY` and `VOLCENGINE_SECRET_KEY` enable the signed official
ECS API. API specifications enrich the anonymous result while preserving
anonymous pricing and availability fields.

## Fallback and risk

The anonymous pricing endpoint and template code are website implementation
details rather than a versioned public contract. If anonymous collection fails,
the scraper uses embedded seed specifications. The seed set has reduced region
and pricing coverage and does not emit `availability`.
