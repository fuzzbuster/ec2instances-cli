package awsutils

import (
	"fmt"
	"strconv"
)

func Floaty(s string) (float64, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("parse float %q: %w", s, err)
	}
	return f, nil
}
