package utils

import (
	"encoding/json"
	"log"
)

// LoadJsonWithBearerToken loads a JSON file from the given URL and unmarshals it into the given value.
// If the bearer token is nil, it will not be added to the request.
//
// The shared req client retries transient network and HTTP failures via
// FetchWithRetry, so a single timeout does not abort the whole scrape.
func LoadJsonWithBearerToken(url string, val any, bearerToken *string) error {
	body, err := FetchWithRetry(url, bearerToken)
	if err != nil {
		return err
	}

	log.Default().Printf("Loaded %d bytes from %s", len(body), url)

	return json.Unmarshal(body, val)
}

// LoadJson loads a JSON file from the given URL and unmarshals it into the given value.
func LoadJson(url string, val any) error {
	return LoadJsonWithBearerToken(url, val, nil)
}
