package utils

import (
	"github.com/anaskhan96/soup"
)

// LoadHTML loads an HTML document from a URL and returns the root node.
//
// It shares the bounded retry policy and pooled req client used by the JSON
// loader.
func LoadHTML(url string) (*soup.Root, error) {
	body, err := FetchWithRetry(url, nil)
	if err != nil {
		return nil, err
	}

	val := soup.HTMLParse(string(body))
	if val.Error != nil {
		return nil, val.Error
	}

	return &val, nil
}
