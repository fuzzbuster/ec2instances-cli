# Hetzner Cloud without credentials

Hetzner's authenticated Cloud API is not used. Public product pages and the
website price API provide the required catalog.

## Data sources

- Cloud product pages under `https://www.hetzner.com/cloud/` for Cost
  Optimized, Regular Performance and General Purpose plans.
- Website price API:
  `https://website-price-api.hetzner.com/api/v1/products/<product-key>`.

The scraper parses each product table for plan name, vCPU, memory, NVMe storage,
architecture and `product-key`. It then queries active price locations and
emits per-datacenter hourly USD prices. Monthly price divided by 730 is used
only when hourly pricing is unavailable.

## Availability

Active price locations with a valid price produce `availability` with
`status: "offered"`, `evidence: "pricing"` and the `ondemand` purchase option.
The `active` flag describes a current price location and does not prove
realtime server capacity.

## Credentials and limits

No Hetzner token is read or required. The current scope is cloud VM plans; it
does not include dedicated servers or GPU products. ARM or Ampere labels map to
`arm64`; other plans map to `x86_64`.

## Failure behavior and risk

A product-page fetch, malformed active price or price API failure fails the
scrape. Website DOM and the price API are not guaranteed as stable public
contracts, and there is no static fallback.
