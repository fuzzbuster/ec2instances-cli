# Instance availability

Anonymous provider sources expose different levels of availability evidence.
Every instance may contain an additive `availability` object keyed by region:

```json
{
  "availability": {
    "ap-nanjing": {
      "status": "available",
      "evidence": "realtime",
      "purchase_options": {
        "ondemand": "available",
        "prepaid": "available",
        "spot": "sold_out"
      },
      "zones": {
        "ap-nanjing-1": {
          "status": "available",
          "purchase_options": {
            "ondemand": "available",
            "prepaid": "available",
            "spot": "sold_out"
          }
        }
      }
    }
  }
}
```

Regions without zone-level data omit `zones`. The regional status summarizes
the available purchase options and zones.

## Status values

- `available`: the source says the instance can be created or deployed.
- `limited`: the source explicitly reports limited availability.
- `sold_out`: the source explicitly reports that the instance is sold out.
- `offered`: a product or price exists, but the source does not prove current
  capacity.
- `unknown`: the source returned a status the scraper could not classify.

An absent region or purchase option means it was not observed. It does not mean
`sold_out`.

## Evidence values

- `realtime`: current sales or inventory status returned by the provider.
- `catalog`: an official product catalog or regional capability declaration.
- `pricing`: a public price or calculator product record.

Always interpret `status` together with `evidence`. In particular,
`catalog/available` does not guarantee instantaneous capacity, and
`pricing/offered` only proves that the provider publishes the product or price.
Scraped availability is a point-in-time reference and does not guarantee that a
subsequent create request will succeed.

## Purchase options

The normalized purchase option keys are:

- `ondemand`
- `prepaid`
- `spot`
- `preemptible`
- `lowpriority`
- `reserved`

Providers only emit options supported by their anonymous source.

## Provider coverage

| Provider | Evidence | Granularity |
| --- | --- | --- |
| Tencent Cloud | `realtime` | Region, zone and On-Demand/Prepaid/Spot |
| Vultr | `catalog` | Region and On-Demand/Preemptible |
| Linode | `catalog` | Region and On-Demand |
| DigitalOcean | `catalog` | Datacenter and full/limited On-Demand |
| Volcengine | `catalog` | Region, zone and On-Demand/Prepaid |
| AWS | `pricing` | Region and On-Demand |
| Azure | `pricing` | Region and published purchase options |
| Alibaba Cloud | `pricing` | Region and On-Demand/Prepaid |
| Huawei Cloud | `pricing` | Region and On-Demand/Prepaid/Reserved |
| Hetzner | `pricing` | Datacenter and On-Demand |

Google Compute Engine is excluded because the required catalog and compute APIs
do not support anonymous access.

The existing `regions`, `pricing` and `availability_zones` fields remain
unchanged for compatibility. New consumers should use `availability` when they
need availability semantics.
