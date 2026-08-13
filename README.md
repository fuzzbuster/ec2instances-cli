# ec2instances CLI

`ec2instances` collects cloud virtual-machine specifications, regional
availability, and pricing as JSON. This `scraper-cli` branch is intended for AI
agents and automation. It does not include the original website, database
services, deployment code, or container images.

This repository is now maintained as an independent CLI project. Its scraper
design was inspired by the
[vantage-sh/ec2instances.info](https://github.com/vantage-sh/ec2instances.info)
scraper.

## Install

Download the archive for Linux or macOS from
[GitHub Releases](https://github.com/fuzzbuster/ec2instances-cli/releases), or
build from source with Go 1.26:

```sh
make build-local
```

Release tags use the `scraper-v<semver>` format. Published targets are:

- Linux amd64 and arm64
- macOS amd64 and arm64

Each release includes `checksums.txt`.

## Data snapshots

An anonymous data snapshot is published on the 15th of each month at 00:00 UTC.
Snapshot releases use `data-YYYY-MM-DD` tags and contain
`ec2instances-data-YYYY-MM-DD.tar.gz` plus `checksums.txt`. The archive preserves
the CLI output paths and contains the raw `instances.json` files for AWS, Azure,
Alibaba Cloud, Tencent Cloud, Volcengine, Huawei Cloud, Vultr, Linode,
DigitalOcean, and Hetzner.

GCP is excluded because its required catalog and compute APIs do not support
anonymous access. Snapshot availability describes the evidence observed at
scrape time, not guaranteed realtime capacity; see
[instance availability](docs/availability.md).

## Usage

```text
ec2instances [--json] providers
ec2instances [--json] version
ec2instances [--json] scrape --providers <names> [--output-dir <path>] [--request-timeout <duration>] [--request-attempts <count>]
```

Always select providers explicitly. The CLI does not start an implicit
full-cloud scrape.

```sh
ec2instances --json providers
ec2instances --json scrape \
  --providers tencentcloud,volcengine \
  --request-timeout 2m \
  --request-attempts 3 \
  --output-dir ./output
```

Provider selection uses this precedence:

1. `--providers`
2. `EC2INSTANCES_PROVIDERS`
3. `ALLOWED_SERVICES` for compatibility with the upstream scraper

The output directory uses this precedence:

1. `--output-dir`
2. `EC2INSTANCES_OUTPUT_DIR`
3. `./output`

HTTP request settings use this precedence:

1. `--request-timeout` / `--request-attempts`
2. `EC2INSTANCES_REQUEST_TIMEOUT` / `EC2INSTANCES_REQUEST_ATTEMPTS`
3. `15m` per request attempt / 6 maximum attempts

The timeout applies to each HTTP attempt, not to a complete provider scrape.
Durations use Go syntax such as `30s`, `2m`, or `15m`. Proxy settings use the
standard `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` environment variables.

Each dataset is written as JSON and as matching `.gz` and `.br` files.

## Providers

| Provider | Name | Environment |
| --- | --- | --- |
| Amazon EC2 | `aws` | Optional: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN` |
| Azure Virtual Machines | `azure` | Optional: `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_SUBSCRIPTION_ID` |
| Google Compute Engine | `gcp` | `GCP_PROJECT_ID`, `GCP_CLIENT_EMAIL`, `GCP_PRIVATE_KEY` |
| Alibaba Cloud ECS | `alicloud` | Optional: `ALICLOUD_ACCESS_KEY`, `ALICLOUD_SECRET_KEY` |
| Tencent Cloud CVM | `tencentcloud` | Optional: `TENCENTCLOUD_SECRET_ID`, `TENCENTCLOUD_SECRET_KEY` |
| Volcengine ECS | `volcengine` | Optional: `VOLCENGINE_ACCESS_KEY`, `VOLCENGINE_SECRET_KEY` |
| Huawei Cloud ECS | `huaweicloud` | Optional Huawei access keys, project ID, and region |
| Vultr | `vultr` | None |
| Linode | `linode` | None |
| DigitalOcean | `digitalocean` | None |
| Hetzner Cloud | `hetzner` | None |

Depending on the provider, optional credentials enrich public data or select an
official API path.

AWS credentials only need the `ec2:DescribeInstanceTypes` IAM permission. AWS
output includes instance specifications and Linux On-Demand pricing, but does
not include Reserved or Spot pricing, interruption rates, or Spot savings
estimates. The EC2 API request uses the built-in SigV4 Query client and does not
depend on the AWS SDK.
Azure credentials add Compute SKU capability fields; base specifications and
regional pricing do not require credentials.

See [Azure setup](docs/setting-up-azure.md) for optional SKU enrichment and
[GCP setup](docs/setting-up-gcp.md) for credential creation.

Anonymous datasets expose normalized availability evidence where their public
sources allow it. See [instance availability](docs/availability.md) for the
`availability` schema and the distinction between realtime, catalog and pricing
evidence. Existing `regions`, `pricing` and `availability_zones` fields remain
unchanged.

Provider-specific anonymous data source documentation:

- [AWS](docs/anonymous-aws.md)
- [Azure](docs/anonymous-azure.md)
- [GCP credential requirement](docs/anonymous-gcp.md)
- [Alibaba Cloud](docs/anonymous-alicloud.md)
- [Tencent Cloud](docs/anonymous-tencentcloud.md)
- [Volcengine](docs/anonymous-volcengine.md)
- [Huawei Cloud](docs/anonymous-huaweicloud.md)
- [Vultr](docs/anonymous-vultr.md)
- [Linode](docs/anonymous-linode.md)
- [DigitalOcean](docs/anonymous-digitalocean.md)
- [Hetzner](docs/anonymous-hetzner.md)

## Warning Notifications

Scrape warnings are always printed to stderr. They can also be sent to optional
notification providers when the following environment variables are set:

| Provider | Environment |
| --- | --- |
| Slack | `SLACK_WEBHOOK_URL` |
| Feishu | `FEISHU_WEBHOOK_URL`; optional `FEISHU_SECRET` for signed webhooks |
| ntfy | `NTFY_URL`; optional `NTFY_TOKEN` for bearer auth or `NTFY_AUTH_HEADER` for a raw `Authorization` header |
| Bark | `BARK_URL` server base URL, for example `https://api.day.app`; `BARK_DEVICE_KEY` |

Notification requests always use their independent 5 second timeout and are not
affected by scrape request settings. Delivery failures are logged and do not
fail the scrape.

## Agent Contract

With `--json`, stdout contains exactly one JSON object. Progress logs go to
stderr.

Successful scrape:

```json
{
  "status": "ok",
  "command": "scrape",
  "output_dir": "/absolute/path/output",
  "succeeded": ["tencentcloud"],
  "failed": [],
  "partial": false
}
```

Providers run independently. If one fails, the others finish and successful
files remain in the output directory:

```json
{
  "status": "error",
  "command": "scrape",
  "output_dir": "/absolute/path/output",
  "succeeded": ["tencentcloud"],
  "failed": [{"name": "aws", "error": "request failed"}],
  "partial": true,
  "error": "one or more providers failed"
}
```

Exit codes:

- `0`: success
- `1`: provider, network, parsing, or write failure
- `2`: invalid command, provider, or credential configuration

Treat any result with `partial: true` as an incomplete snapshot.

## Development

```sh
make check
```

`make check` runs formatting checks, vet, tests, build, and skill metadata
validation.

The CLI uses only the Go standard library for command parsing. Scraper output
schemas remain compatible with the original compute-instance datasets.

## License

MIT. See [LICENSE](LICENSE). Original project copyright and attribution are
preserved.
