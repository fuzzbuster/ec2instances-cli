package aws

import "testing"

func TestLoadOptionalEC2DescriptionsWithoutCredentials(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	if got := loadOptionalEC2Descriptions(); got != nil {
		t.Fatalf("descriptions = %v, want nil without credentials", got)
	}
}

func TestLoadOptionalEC2DescriptionsRequiresBothCredentials(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	if got := loadOptionalEC2Descriptions(); got != nil {
		t.Fatalf("descriptions = %v, want nil with partial credentials", got)
	}
}
