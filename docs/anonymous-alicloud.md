# Alibaba Cloud ECS without credentials

Alibaba Cloud ECS pricing can be collected without access keys.

## Data sources

- International pricing CDN:
  `https://g.alicdn.com/aliyun/ecs-price-info-intl/2.0.8/price/download/instancePrice.json`.
- China pricing CDN:
  `https://g.alicdn.com/aliyun/ecs-price-info/2.0.8/price/download/instancePrice.json`.

Each pricing key contains region, instance type, network, operating system and
I/O mode. The scraper builds regional On-Demand prices from these keys. In
anonymous mode, vCPU and memory are derived from the documented ECS family and
size naming scheme.

## Availability

Regional price entries produce `availability` with `status: "offered"` and
`evidence: "pricing"`. Hourly periods map to `ondemand`; monthly or yearly
periods map to `prepaid`. The CDN does not expose zone inventory or current
sell-out status.

## Optional credentials

`ALICLOUD_ACCESS_KEY` and `ALICLOUD_SECRET_KEY` enable the signed
`DescribeInstanceTypes` RPC call. Its authoritative specifications replace
name-derived values when a matching instance type is available.

## Limits and failure behavior

Name-derived specifications may be incomplete for unusual or newly introduced
families. One pricing CDN may fail while the other still produces output; the
scrape fails only when both sources fail.
