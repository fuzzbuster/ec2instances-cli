package utils

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/imroc/req/v3"
)

type HTTPConfig struct {
	RequestTimeout time.Duration
	MaxAttempts    int
}

var (
	retryBaseDelay = 2 * time.Second
	retryMaxDelay  = 30 * time.Second

	httpConfigMu      sync.RWMutex
	currentHTTPConfig = DefaultHTTPConfig()
	sharedHTTPClient  = newRetryingHTTPClient(currentHTTPConfig)
)

func DefaultHTTPConfig() HTTPConfig {
	return HTTPConfig{
		RequestTimeout: 15 * time.Minute,
		MaxAttempts:    6,
	}
}

func ConfigureHTTP(config HTTPConfig) error {
	if config.RequestTimeout <= 0 {
		return fmt.Errorf("request timeout must be greater than zero")
	}
	if config.MaxAttempts < 1 {
		return fmt.Errorf("request attempts must be at least 1")
	}

	client := newRetryingHTTPClient(config)
	httpConfigMu.Lock()
	currentHTTPConfig = config
	sharedHTTPClient = client
	httpConfigMu.Unlock()
	return nil
}

func CurrentHTTPConfig() HTTPConfig {
	httpConfigMu.RLock()
	defer httpConfigMu.RUnlock()
	return currentHTTPConfig
}

func RetryingHTTPClient() *req.Client {
	httpConfigMu.RLock()
	defer httpConfigMu.RUnlock()
	return sharedHTTPClient
}

func NewHTTPClient() *req.Client {
	return newHTTPClient(CurrentHTTPConfig())
}

func retryableStatus(code int) bool {
	return code == http.StatusTooManyRequests ||
		code == http.StatusNotFound ||
		code >= 500
}

func backoffFor(attempt int) time.Duration {
	delay := retryBaseDelay << (attempt - 1)
	if delay > retryMaxDelay || delay <= 0 {
		delay = retryMaxDelay
	}
	return delay
}

func newHTTPClient(config HTTPConfig) *req.Client {
	client := req.C().
		SetTimeout(config.RequestTimeout).
		DisableAutoDecode().
		SetProxy(http.ProxyFromEnvironment)

	client.Transport.
		SetMaxIdleConns(200).
		SetMaxConnsPerHost(50).
		SetIdleConnTimeout(90 * time.Second).
		SetTLSHandshakeTimeout(30 * time.Second).
		SetResponseHeaderTimeout(60 * time.Second).
		SetExpectContinueTimeout(time.Second)
	client.Transport.MaxIdleConnsPerHost = 50

	return client
}

func newRetryingHTTPClient(config HTTPConfig) *req.Client {
	client := newHTTPClient(config).
		SetCommonRetryCount(config.MaxAttempts - 1).
		SetCommonRetryCondition(func(resp *req.Response, err error) bool {
			if err != nil {
				return true
			}
			return resp != nil && resp.Response != nil && retryableStatus(resp.StatusCode)
		}).
		SetCommonRetryInterval(func(_ *req.Response, attempt int) time.Duration {
			return backoffFor(attempt)
		}).
		SetCommonRetryHook(func(resp *req.Response, err error) {
			if resp == nil || resp.Request == nil {
				return
			}
			retryErr := err
			if retryErr == nil && resp.Response != nil {
				retryErr = &httpStatusError{
					url:  resp.Request.RawURL,
					code: resp.StatusCode,
					body: resp.String(),
				}
			}
			log.Printf(
				"Failed to load %s, retrying in %s... (attempt %d/%d): %v",
				resp.Request.RawURL,
				backoffFor(resp.Request.RetryAttempt),
				resp.Request.RetryAttempt+1,
				config.MaxAttempts,
				retryErr,
			)
		})
	return client
}

// FetchWithRetry performs a GET with bounded retry and returns the response body.
func FetchWithRetry(urlStr string, bearerToken *string) ([]byte, error) {
	if _, err := url.ParseRequestURI(urlStr); err != nil {
		return nil, err
	}

	request := RetryingHTTPClient().R()
	if bearerToken != nil {
		request.SetBearerAuthToken(*bearerToken)
	}
	resp, err := request.Get(urlStr)
	return responseBody(urlStr, resp, err)
}

// PostFormWithRetry performs a form POST with the same retry policy as FetchWithRetry.
func PostFormWithRetry(urlStr string, data url.Values) ([]byte, error) {
	if _, err := url.ParseRequestURI(urlStr); err != nil {
		return nil, err
	}

	resp, err := RetryingHTTPClient().R().
		SetFormDataFromValues(data).
		Post(urlStr)
	return responseBody(urlStr, resp, err)
}

func responseBody(urlStr string, resp *req.Response, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{
			url:  urlStr,
			code: resp.StatusCode,
			body: resp.String(),
		}
	}
	return resp.Bytes(), nil
}

type httpStatusError struct {
	url  string
	code int
	body string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("status code %d for url %s: %s", e.code, e.url, e.body)
}
