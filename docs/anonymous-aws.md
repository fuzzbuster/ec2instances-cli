# AWS EC2 without credentials

AWS EC2 scraping works without AWS credentials.

## Data sources

- Official EC2 instance-type Markdown tables under
  `https://docs.aws.amazon.com/ec2/latest/instancetypes/`.
- Compact public Linux On-Demand pricing:
  `https://b0.p.awsstatic.com/pricing/2.0/meteredUnitMaps/ec2/USD/current/ec2.json`.

The scraper parses vCPU, memory, processor, architecture, cores and accelerator
fields from the Markdown tables, then joins prices by instance type and region.
It does not download the legacy multi-hundred-megabyte Bulk Offer catalog.

## Availability

An instance with a public Linux On-Demand price receives regional
`availability` with `status: "offered"`, `evidence: "pricing"` and the
`ondemand` purchase option. This proves that AWS publishes a regional price; it
does not report current capacity or Availability Zone inventory.

## Optional credentials

`AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` enable
`DescribeInstanceTypes`; `AWS_SESSION_TOKEN` is also supported. The API result
adds fields such as network, EBS, storage and Nitro capabilities. Credentials
only need `ec2:DescribeInstanceTypes`.

## Limits and failure behavior

Anonymous output contains Linux On-Demand prices. Reserved and Spot prices,
interruption rates and Spot savings are not collected. Failure to load a
required specification document or the compact pricing file fails the scrape;
there is no static fallback.
