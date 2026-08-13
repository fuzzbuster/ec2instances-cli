package aws

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/imroc/req/v3"

	ec2Internal "github.com/fuzzbuster/ec2instances-cli/aws/ec2"
	"github.com/fuzzbuster/ec2instances-cli/utils"
)

const ec2APIVersion = "2016-11-15"

var (
	ec2APIRetryBaseDelay = 100 * time.Millisecond
	ec2APIRetryMaxDelay  = 20 * time.Second
)

type ec2Credentials struct {
	accessKeyID     string
	secretAccessKey string
	sessionToken    string
}

type ec2APIClient struct {
	httpClient  *req.Client
	credentials ec2Credentials
	endpoint    func(string) string
	now         func() time.Time
}

type ec2APIError struct {
	Code      string
	Message   string
	RequestID string
	Status    int
}

func (e *ec2APIError) Error() string {
	return fmt.Sprintf("EC2 API %s: %s (status %d, request %s)", e.Code, e.Message, e.Status, e.RequestID)
}

type ec2APIErrorResponse struct {
	Errors []struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	} `xml:"Errors>Error"`
	RequestID string `xml:"RequestID"`
}

type sigV4Details struct {
	canonicalRequest string
	stringToSign     string
	authorization    string
	signedHeaders    string
}

func newEC2APIClient() *ec2APIClient {
	return &ec2APIClient{
		httpClient: utils.NewHTTPClient(),
		credentials: ec2Credentials{
			accessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
			secretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
			sessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
		},
		endpoint: func(region string) string {
			return "https://ec2." + region + ".amazonaws.com/"
		},
		now: time.Now,
	}
}

func (c *ec2APIClient) describeInstanceTypes(
	region string,
	pushPage func([]ec2Internal.APIInstanceTypeInfo),
) (int, error) {
	nextToken := ""
	pages := 0
	for {
		output, err := c.describeInstanceTypesPage(region, nextToken)
		if err != nil {
			return pages, err
		}
		pages++
		pushPage(output.InstanceTypes)
		if output.NextToken == "" {
			return pages, nil
		}
		nextToken = output.NextToken
	}
}

func (c *ec2APIClient) describeInstanceTypesPage(region, nextToken string) (*ec2Internal.APIDescribeInstanceTypesResponse, error) {
	bodyValues := url.Values{
		"Action":     {"DescribeInstanceTypes"},
		"MaxResults": {"100"},
		"Version":    {ec2APIVersion},
	}
	if nextToken != "" {
		bodyValues.Set("NextToken", nextToken)
	}
	body := []byte(bodyValues.Encode())
	endpoint := c.endpoint(region)
	maxAttempts := utils.CurrentHTTPConfig().MaxAttempts

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		headers, _, err := signEC2Request(c.credentials, region, endpoint, body, c.now().UTC())
		if err != nil {
			return nil, err
		}

		resp, err := c.httpClient.R().
			SetHeaders(headers).
			SetBodyBytes(body).
			Post(endpoint)
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				var output ec2Internal.APIDescribeInstanceTypesResponse
				if err := xml.Unmarshal(resp.Bytes(), &output); err != nil {
					return nil, fmt.Errorf("decode DescribeInstanceTypes response: %w", err)
				}
				return &output, nil
			}
			err = decodeEC2APIError(resp.StatusCode, resp.Bytes())
		}
		lastErr = err
		if attempt == maxAttempts || !isRetryableEC2Error(err) {
			return nil, err
		}
		time.Sleep(ec2APIBackoff(attempt))
	}
	return nil, lastErr
}

func signEC2Request(credentials ec2Credentials, region, endpoint string, body []byte, now time.Time) (map[string]string, sigV4Details, error) {
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil {
		return nil, sigV4Details{}, fmt.Errorf("parse EC2 endpoint: %w", err)
	}
	if parsedEndpoint.Host == "" {
		return nil, sigV4Details{}, fmt.Errorf("EC2 endpoint has no host")
	}

	const contentType = "application/x-www-form-urlencoded"
	amzDate := now.UTC().Format("20060102T150405Z")
	date := now.UTC().Format("20060102")
	payloadHash := sha256Hex(body)

	canonicalHeaders := "content-type:" + contentType + "\n" +
		"host:" + parsedEndpoint.Host + "\n" +
		"x-amz-date:" + amzDate + "\n"
	signedHeaders := "content-type;host;x-amz-date"
	if credentials.sessionToken != "" {
		canonicalHeaders += "x-amz-security-token:" + credentials.sessionToken + "\n"
		signedHeaders += ";x-amz-security-token"
	}

	canonicalRequest := strings.Join([]string{
		http.MethodPost,
		"/",
		"",
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	scope := date + "/" + region + "/ec2/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	dateKey := hmacSHA256([]byte("AWS4"+credentials.secretAccessKey), date)
	regionKey := hmacSHA256(dateKey, region)
	serviceKey := hmacSHA256(regionKey, "ec2")
	signingKey := hmacSHA256(serviceKey, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))
	authorization := "AWS4-HMAC-SHA256 Credential=" + credentials.accessKeyID + "/" + scope +
		", SignedHeaders=" + signedHeaders + ", Signature=" + signature

	headers := map[string]string{
		"Authorization": authorization,
		"Content-Type":  contentType,
		"X-Amz-Date":    amzDate,
	}
	if credentials.sessionToken != "" {
		headers["X-Amz-Security-Token"] = credentials.sessionToken
	}
	return headers, sigV4Details{
		canonicalRequest: canonicalRequest,
		stringToSign:     stringToSign,
		authorization:    authorization,
		signedHeaders:    signedHeaders,
	}, nil
}

func decodeEC2APIError(status int, body []byte) error {
	var response ec2APIErrorResponse
	if err := xml.Unmarshal(body, &response); err != nil || len(response.Errors) == 0 {
		return &ec2APIError{
			Code:    "HTTPError",
			Message: strings.TrimSpace(string(body)),
			Status:  status,
		}
	}
	return &ec2APIError{
		Code:      response.Errors[0].Code,
		Message:   response.Errors[0].Message,
		RequestID: response.RequestID,
		Status:    status,
	}
}

func isRetryableEC2Error(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *ec2APIError
	if errors.As(err, &apiErr) {
		if apiErr.Status == http.StatusTooManyRequests || apiErr.Status >= 500 {
			return true
		}
		switch apiErr.Code {
		case "RequestLimitExceeded", "RateLimitExceeded", "Throttling", "ThrottlingException", "RequestThrottled":
			return true
		default:
			return false
		}
	}
	return true
}

func isEC2RateLimitError(err error) bool {
	var apiErr *ec2APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.Code {
	case "RequestLimitExceeded", "RateLimitExceeded", "Throttling", "ThrottlingException", "RequestThrottled":
		return true
	default:
		return false
	}
}

func ec2APIBackoff(attempt int) time.Duration {
	delay := ec2APIRetryBaseDelay << (attempt - 1)
	if delay > ec2APIRetryMaxDelay || delay <= 0 {
		return ec2APIRetryMaxDelay
	}
	return delay
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
