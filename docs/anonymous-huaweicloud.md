# Huawei Cloud ECS without credentials

Huawei Cloud ECS scraping uses the public pricing calculator without AK/SK.

## Data sources

- Calculator menu and region discovery:
  `https://portal.huaweicloud.com/api/calculator/rest/cbc/portalcalculatornodeservice/v4/api/menuInfo`.
- Per-region ECS products:
  `https://portal.huaweicloud.com/api/calculator/rest/cbc/portalcalculatornodeservice/v4/api/productInfo`.

The scraper selects the ECS menu, visits every advertised region and merges
Linux VM records. The result includes vCPU, memory, architecture, accelerator,
local disk, region and On-Demand, monthly and yearly prices.

## Availability

Calculator plans produce regional `availability` with `status: "offered"` and
`evidence: "pricing"`. `ONDEMAND` maps to `ondemand`, monthly and yearly plans
map to `prepaid`, and `RI` maps to `reserved`. Calculator presence does not
prove current capacity.

## Optional credentials

`HUAWEICLOUD_ACCESS_KEY` and `HUAWEICLOUD_SECRET_KEY`, together with
region-scoped project IDs, enable the official ECS flavor API. Project IDs may
be supplied through `HUAWEICLOUD_PROJECT_ID` and `HUAWEICLOUD_REGION` or the
region-specific variables recognized by the scraper.

## Fallback and risk

Calculator endpoints are public website services and may change without an API
deprecation period. Failed regions are skipped. If all anonymous regions fail
or yield no instances, the scraper uses its smaller embedded seed dataset,
which does not emit `availability`.
