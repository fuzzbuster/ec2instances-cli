package ec2

import "encoding/xml"

// APIInstanceTypeInfo contains only the DescribeInstanceTypes fields consumed
// by the scraper.
type APIInstanceTypeInfo struct {
	InstanceType         string                  `xml:"instanceType"`
	BareMetal            *bool                   `xml:"bareMetal"`
	Hypervisor           string                  `xml:"hypervisor"`
	NitroEnclavesSupport string                  `xml:"nitroEnclavesSupport"`
	ProcessorInfo        *APIProcessorInfo       `xml:"processorInfo"`
	VCpuInfo             *APIVCpuInfo            `xml:"vCpuInfo"`
	NetworkInfo          *APINetworkInfo         `xml:"networkInfo"`
	EbsInfo              *APIEbsInfo             `xml:"ebsInfo"`
	InstanceStorageInfo  *APIInstanceStorageInfo `xml:"instanceStorageInfo"`
	FpgaInfo             *APIFpgaInfo            `xml:"fpgaInfo"`
}

type APIProcessorInfo struct {
	SupportedArchitectures []string `xml:"supportedArchitectures>item"`
}

type APIVCpuInfo struct {
	DefaultCores *int32 `xml:"defaultCores"`
}

type APINetworkInfo struct {
	NetworkPerformance        *string              `xml:"networkPerformance"`
	EnaSupport                string               `xml:"enaSupport"`
	MaximumNetworkInterfaces  *int32               `xml:"maximumNetworkInterfaces"`
	Ipv4AddressesPerInterface *int32               `xml:"ipv4AddressesPerInterface"`
	NetworkCards              []APINetworkCardInfo `xml:"networkCards>item"`
}

type APINetworkCardInfo struct {
	BaselineBandwidthInGbps *float64 `xml:"baselineBandwidthInGbps"`
	PeakBandwidthInGbps     *float64 `xml:"peakBandwidthInGbps"`
}

type APIEbsInfo struct {
	EbsOptimizedInfo *APIEbsOptimizedInfo `xml:"ebsOptimizedInfo"`
}

type APIEbsOptimizedInfo struct {
	BaselineBandwidthInMbps  *int32   `xml:"baselineBandwidthInMbps"`
	BaselineIops             *int32   `xml:"baselineIops"`
	BaselineThroughputInMBps *float64 `xml:"baselineThroughputInMBps"`
	MaximumBandwidthInMbps   *int32   `xml:"maximumBandwidthInMbps"`
	MaximumIops              *int32   `xml:"maximumIops"`
	MaximumThroughputInMBps  *float64 `xml:"maximumThroughputInMBps"`
}

type APIInstanceStorageInfo struct {
	NvmeSupport string        `xml:"nvmeSupport"`
	Disks       []APIDiskInfo `xml:"disks>item"`
}

type APIDiskInfo struct {
	Count    *int32 `xml:"count"`
	SizeInGB *int64 `xml:"sizeInGB"`
	Type     string `xml:"type"`
}

type APIFpgaInfo struct {
	Fpgas []APIFpgaDeviceInfo `xml:"fpgas>item"`
}

type APIFpgaDeviceInfo struct {
	Count *int32 `xml:"count"`
}

type APIDescribeInstanceTypesResponse struct {
	XMLName       xml.Name              `xml:"DescribeInstanceTypesResponse"`
	InstanceTypes []APIInstanceTypeInfo `xml:"instanceTypeSet>item"`
	NextToken     string                `xml:"nextToken"`
	RequestID     string                `xml:"requestId"`
}
