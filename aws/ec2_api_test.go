package aws

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/imroc/req/v3"

	ec2Internal "github.com/fuzzbuster/ec2instances-cli/aws/ec2"
	"github.com/fuzzbuster/ec2instances-cli/utils"
)

const describeInstanceTypesXML = `<?xml version="1.0" encoding="UTF-8"?>
<DescribeInstanceTypesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>request-1</requestId>
  <instanceTypeSet>
    <item>
      <instanceType>m7i.large</instanceType>
      <bareMetal>false</bareMetal>
      <hypervisor>nitro</hypervisor>
      <nitroEnclavesSupport>supported</nitroEnclavesSupport>
      <processorInfo>
        <supportedArchitectures><item>x86_64</item></supportedArchitectures>
      </processorInfo>
      <vCpuInfo><defaultCores>1</defaultCores></vCpuInfo>
      <networkInfo>
        <networkPerformance>Up to 12.5 Gigabit</networkPerformance>
        <enaSupport>required</enaSupport>
        <maximumNetworkInterfaces>3</maximumNetworkInterfaces>
        <ipv4AddressesPerInterface>10</ipv4AddressesPerInterface>
        <networkCards>
          <item>
            <baselineBandwidthInGbps>0.781</baselineBandwidthInGbps>
            <peakBandwidthInGbps>12.5</peakBandwidthInGbps>
          </item>
        </networkCards>
      </networkInfo>
      <ebsInfo>
        <ebsOptimizedInfo>
          <baselineBandwidthInMbps>650</baselineBandwidthInMbps>
          <baselineIops>3600</baselineIops>
          <baselineThroughputInMBps>81.25</baselineThroughputInMBps>
          <maximumBandwidthInMbps>10000</maximumBandwidthInMbps>
          <maximumIops>40000</maximumIops>
          <maximumThroughputInMBps>1250</maximumThroughputInMBps>
        </ebsOptimizedInfo>
      </ebsInfo>
      <instanceStorageInfo>
        <nvmeSupport>required</nvmeSupport>
        <disks><item><count>1</count><sizeInGB>118</sizeInGB><type>ssd</type></item></disks>
      </instanceStorageInfo>
      <fpgaInfo><fpgas><item><count>2</count></item></fpgas></fpgaInfo>
    </item>
  </instanceTypeSet>
  <nextToken>next-page</nextToken>
</DescribeInstanceTypesResponse>`

func TestSignEC2Request(t *testing.T) {
	body := []byte("Action=DescribeInstanceTypes&MaxResults=100&Version=2016-11-15")
	credentials := ec2Credentials{
		accessKeyID:     "AKIDEXAMPLE",
		secretAccessKey: "SECRET",
	}
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	headers, details, err := signEC2Request(
		credentials,
		"us-east-1",
		"https://ec2.us-east-1.amazonaws.com/",
		body,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}

	wantCanonical := "POST\n/\n\n" +
		"content-type:application/x-www-form-urlencoded\n" +
		"host:ec2.us-east-1.amazonaws.com\n" +
		"x-amz-date:20240102T030405Z\n\n" +
		"content-type;host;x-amz-date\n" +
		"654242d7221c8c0ab7c268e148ebaef9888a612ddd31f3028db293df6d2fe2de"
	if details.canonicalRequest != wantCanonical {
		t.Fatalf("canonical request mismatch:\n%s", details.canonicalRequest)
	}
	wantStringToSign := "AWS4-HMAC-SHA256\n20240102T030405Z\n" +
		"20240102/us-east-1/ec2/aws4_request\n" +
		"41c2ccbacb759203bc73151c350163266a96e242db4015f55d49aab5e1639e2b"
	if details.stringToSign != wantStringToSign {
		t.Fatalf("string to sign mismatch:\n%s", details.stringToSign)
	}
	wantAuthorization := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20240102/us-east-1/ec2/aws4_request, " +
		"SignedHeaders=content-type;host;x-amz-date, " +
		"Signature=6ecdd703a15b8bd2549ff3474b76b7e222a32472303890e060c35f3018ad39e0"
	if details.authorization != wantAuthorization {
		t.Fatalf("authorization = %q, want %q", details.authorization, wantAuthorization)
	}
	if headers["X-Amz-Date"] != "20240102T030405Z" {
		t.Fatalf("X-Amz-Date = %q", headers["X-Amz-Date"])
	}
}

func TestSignEC2RequestWithSessionToken(t *testing.T) {
	headers, details, err := signEC2Request(
		ec2Credentials{accessKeyID: "key", secretAccessKey: "secret", sessionToken: "token"},
		"us-east-1",
		"https://ec2.us-east-1.amazonaws.com/",
		[]byte("body"),
		time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if headers["X-Amz-Security-Token"] != "token" {
		t.Fatalf("security token = %q", headers["X-Amz-Security-Token"])
	}
	if details.signedHeaders != "content-type;host;x-amz-date;x-amz-security-token" {
		t.Fatalf("signed headers = %q", details.signedHeaders)
	}
	if !strings.Contains(details.canonicalRequest, "x-amz-security-token:token\n") {
		t.Fatal("security token is not in canonical request")
	}
}

func TestDescribeInstanceTypesPageDecodesFieldsAndNextToken(t *testing.T) {
	var requestBody url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requestBody, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(describeInstanceTypesXML))
	}))
	defer server.Close()

	client := testEC2APIClient(server.URL)
	output, err := client.describeInstanceTypesPage("us-east-1", "previous token")
	if err != nil {
		t.Fatal(err)
	}
	if requestBody.Get("NextToken") != "previous token" {
		t.Fatalf("NextToken = %q", requestBody.Get("NextToken"))
	}
	if output.NextToken != "next-page" || len(output.InstanceTypes) != 1 {
		t.Fatalf("unexpected output: %#v", output)
	}
	instance := output.InstanceTypes[0]
	if instance.InstanceType != "m7i.large" || instance.ProcessorInfo.SupportedArchitectures[0] != "x86_64" {
		t.Fatalf("unexpected instance identity: %#v", instance)
	}
	if *instance.VCpuInfo.DefaultCores != 1 ||
		*instance.NetworkInfo.MaximumNetworkInterfaces != 3 ||
		*instance.NetworkInfo.NetworkCards[0].PeakBandwidthInGbps != 12.5 {
		t.Fatalf("unexpected compute/network fields: %#v", instance)
	}
	if *instance.EbsInfo.EbsOptimizedInfo.MaximumIops != 40000 ||
		*instance.InstanceStorageInfo.Disks[0].SizeInGB != 118 ||
		*instance.FpgaInfo.Fpgas[0].Count != 2 {
		t.Fatalf("unexpected storage/FPGA fields: %#v", instance)
	}
}

func TestDescribeInstanceTypesFollowsNextToken(t *testing.T) {
	var requestBodies []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		requestBodies = append(requestBodies, values)
		if len(requestBodies) == 1 {
			_, _ = w.Write([]byte(`<DescribeInstanceTypesResponse><instanceTypeSet><item><instanceType>m7i.large</instanceType></item></instanceTypeSet><nextToken>page-2</nextToken></DescribeInstanceTypesResponse>`))
			return
		}
		_, _ = w.Write([]byte(`<DescribeInstanceTypesResponse><instanceTypeSet><item><instanceType>c7i.large</instanceType></item></instanceTypeSet></DescribeInstanceTypesResponse>`))
	}))
	defer server.Close()

	var instanceTypes []string
	pages, err := testEC2APIClient(server.URL).describeInstanceTypes("us-east-1", func(instances []ec2Internal.APIInstanceTypeInfo) {
		for _, instance := range instances {
			instanceTypes = append(instanceTypes, instance.InstanceType)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 || len(requestBodies) != 2 {
		t.Fatalf("pages/requests = %d/%d, want 2/2", pages, len(requestBodies))
	}
	if requestBodies[0].Get("NextToken") != "" || requestBodies[1].Get("NextToken") != "page-2" {
		t.Fatalf("pagination bodies = %#v", requestBodies)
	}
	if strings.Join(instanceTypes, ",") != "m7i.large,c7i.large" {
		t.Fatalf("instance types = %v", instanceTypes)
	}
}

func TestDescribeInstanceTypesPageRetriesTransientFailures(t *testing.T) {
	shrinkEC2APIRetry(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.WriteHeader(http.StatusInternalServerError)
		case 3:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`<Response><Errors><Error><Code>RequestLimitExceeded</Code><Message>slow down</Message></Error></Errors><RequestID>request-2</RequestID></Response>`))
		default:
			_, _ = w.Write([]byte(`<DescribeInstanceTypesResponse><instanceTypeSet></instanceTypeSet></DescribeInstanceTypesResponse>`))
		}
	}))
	defer server.Close()

	_, err := testEC2APIClient(server.URL).describeInstanceTypesPage("us-east-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 4 {
		t.Fatalf("calls = %d, want 4", calls.Load())
	}
}

func TestDescribeInstanceTypesPageRetriesNetworkError(t *testing.T) {
	shrinkEC2APIRetry(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("response writer does not support hijacking")
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_ = conn.Close()
			return
		}
		_, _ = w.Write([]byte(`<DescribeInstanceTypesResponse/>`))
	}))
	defer server.Close()

	_, err := testEC2APIClient(server.URL).describeInstanceTypesPage("us-east-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}

func TestDescribeInstanceTypesPageDoesNotRetryPermanentError(t *testing.T) {
	shrinkEC2APIRetry(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<Response><Errors><Error><Code>UnauthorizedOperation</Code><Message>denied</Message></Error></Errors><RequestID>request-3</RequestID></Response>`))
	}))
	defer server.Close()

	_, err := testEC2APIClient(server.URL).describeInstanceTypesPage("us-east-1", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
	if !strings.Contains(err.Error(), "UnauthorizedOperation") || !strings.Contains(err.Error(), "request-3") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "AKID") {
		t.Fatalf("credentials leaked in error: %v", err)
	}
}

func TestDescribeInstanceTypesPageUsesConfiguredAttemptLimit(t *testing.T) {
	shrinkEC2APIRetry(t)
	original := utils.CurrentHTTPConfig()
	config := original
	config.MaxAttempts = 2
	if err := utils.ConfigureHTTP(config); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := utils.ConfigureHTTP(original); err != nil {
			t.Errorf("restore HTTP config: %v", err)
		}
	})

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if _, err := testEC2APIClient(server.URL).describeInstanceTypesPage("us-east-1", ""); err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}

func testEC2APIClient(endpoint string) *ec2APIClient {
	return &ec2APIClient{
		httpClient: req.C().SetTimeout(time.Second).DisableAutoDecode(),
		credentials: ec2Credentials{
			accessKeyID:     "AKID",
			secretAccessKey: "secret",
		},
		endpoint: func(string) string { return endpoint },
		now: func() time.Time {
			return time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
		},
	}
}

func shrinkEC2APIRetry(t *testing.T) {
	t.Helper()
	originalBase, originalMax := ec2APIRetryBaseDelay, ec2APIRetryMaxDelay
	ec2APIRetryBaseDelay = time.Millisecond
	ec2APIRetryMaxDelay = 2 * time.Millisecond
	t.Cleanup(func() {
		ec2APIRetryBaseDelay = originalBase
		ec2APIRetryMaxDelay = originalMax
	})
}
