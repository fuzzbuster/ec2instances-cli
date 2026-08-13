# Tencent Cloud CVM without credentials

Without a complete credential pair, Tencent Cloud CVM scraping uses the
purchase workbench endpoints and does not require a Secret ID or Secret Key.

## Data sources

- Region discovery:
  `https://workbench.cloud.tencent.com/cgi/area/queryCvmRegion`.
- Per-region and per-zone configuration:
  `https://workbench.cloud.tencent.com/cgi/api?i=cvm/DescribeZoneInstanceConfigInfos&uin=&region=<region>`.

The scraper requests Linux configurations for prepaid, hourly and Spot billing.
It aggregates instance specifications, CPU model, local disks, prices,
sellable availability zones and regional coverage. Duplicate zone records use
the lowest observed price for each price type.

## Availability

The purchase response reports availability for each instance type, zone and
billing mode. The scraper preserves these values under `availability` with
`evidence: "realtime"`:

- `POSTPAID_BY_HOUR` maps to `ondemand`.
- `PREPAID` maps to `prepaid`.
- `SPOTPAID` maps to `spot`.
- `SELL` maps to `available`.
- `SOLD_OUT` maps to `sold_out`.

The regional and zone status is `available` when at least one purchase option
is sellable. Sold-out options remain in the output so consumers can distinguish
them from options that were not observed. The legacy `availability_zones`
field still lists zones where at least one purchase option is sellable.

## Optional credentials

When both `TENCENTCLOUD_SECRET_ID` and `TENCENTCLOUD_SECRET_KEY` are set, the
scraper switches to the official `DescribeInstanceTypeConfigs` path. This path
provides official specifications but does not merge the anonymous prices,
sellable availability zones, CPU model or local disk fields, and therefore does
not emit realtime `availability`.

## Fallback and risk

The workbench endpoints are public but are not a documented stable API
contract. If they fail or return no instances, the scraper falls back to its
embedded seed specifications. Seed output is intentionally less complete and
does not represent real-time inventory or emit `availability`.
