package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/fuzzbuster/ec2instances-cli/alicloud"
	"github.com/fuzzbuster/ec2instances-cli/aws"
	"github.com/fuzzbuster/ec2instances-cli/azure"
	"github.com/fuzzbuster/ec2instances-cli/digitalocean"
	"github.com/fuzzbuster/ec2instances-cli/gcp"
	"github.com/fuzzbuster/ec2instances-cli/hetzner"
	"github.com/fuzzbuster/ec2instances-cli/huaweicloud"
	"github.com/fuzzbuster/ec2instances-cli/linode"
	"github.com/fuzzbuster/ec2instances-cli/tencentcloud"
	"github.com/fuzzbuster/ec2instances-cli/volcengine"
	"github.com/fuzzbuster/ec2instances-cli/vultr"
)

type provider struct {
	Name        string
	RequiredEnv []string
	OptionalEnv []string
	Run         func() error
}

type providerInfo struct {
	Name        string   `json:"name"`
	RequiredEnv []string `json:"required_env"`
	OptionalEnv []string `json:"optional_env"`
}

type providerFailure struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

var providers = []provider{
	{
		Name:        "aws",
		OptionalEnv: []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"},
		Run:         aws.DoAwsScraping,
	},
	{
		Name: "azure",
		OptionalEnv: []string{
			"AZURE_TENANT_ID",
			"AZURE_CLIENT_ID",
			"AZURE_CLIENT_SECRET",
			"AZURE_SUBSCRIPTION_ID",
		},
		Run: azure.DoAzureScraping,
	},
	{
		Name:        "gcp",
		RequiredEnv: []string{"GCP_PROJECT_ID", "GCP_CLIENT_EMAIL", "GCP_PRIVATE_KEY"},
		Run:         gcp.DoGCPScraping,
	},
	{
		Name:        "alicloud",
		OptionalEnv: []string{"ALICLOUD_ACCESS_KEY", "ALICLOUD_SECRET_KEY"},
		Run:         alicloud.DoAlicloudScraping,
	},
	{
		Name:        "tencentcloud",
		OptionalEnv: []string{"TENCENTCLOUD_SECRET_ID", "TENCENTCLOUD_SECRET_KEY"},
		Run:         tencentcloud.DoTencentcloudScraping,
	},
	{
		Name:        "volcengine",
		OptionalEnv: []string{"VOLCENGINE_ACCESS_KEY", "VOLCENGINE_SECRET_KEY"},
		Run:         volcengine.DoVolcengineScraping,
	},
	{
		Name: "huaweicloud",
		OptionalEnv: []string{
			"HUAWEICLOUD_ACCESS_KEY",
			"HUAWEICLOUD_SECRET_KEY",
			"HUAWEICLOUD_PROJECT_ID",
			"HUAWEICLOUD_REGION",
		},
		Run: huaweicloud.DoHuaweicloudScraping,
	},
	{Name: "vultr", Run: vultr.DoVultrScraping},
	{Name: "linode", Run: linode.DoLinodeScraping},
	{Name: "digitalocean", Run: digitalocean.DoDigitalOceanScraping},
	{Name: "hetzner", Run: hetzner.DoHetznerScraping},
}

func selectProviders(value string) ([]provider, error) {
	if strings.TrimSpace(value) == "" {
		return nil, fmt.Errorf("at least one provider must be specified")
	}

	requested := make(map[string]struct{})
	for _, name := range strings.Split(value, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			requested[name] = struct{}{}
		}
	}

	selected := make([]provider, 0, len(requested))
	for _, candidate := range providers {
		if _, ok := requested[candidate.Name]; ok {
			selected = append(selected, candidate)
			delete(requested, candidate.Name)
		}
	}
	if len(requested) > 0 {
		unknown := make([]string, 0, len(requested))
		for name := range requested {
			unknown = append(unknown, name)
		}
		sort.Strings(unknown)
		return nil, fmt.Errorf("unknown provider: %s", strings.Join(unknown, ","))
	}
	return selected, nil
}

func listProviderInfo() []providerInfo {
	info := make([]providerInfo, 0, len(providers))
	for _, candidate := range providers {
		required := append([]string{}, candidate.RequiredEnv...)
		optional := append([]string{}, candidate.OptionalEnv...)
		info = append(info, providerInfo{
			Name:        candidate.Name,
			RequiredEnv: required,
			OptionalEnv: optional,
		})
	}
	return info
}

func validateCredentials(selected []provider) error {
	var missing []string
	for _, selectedProvider := range selected {
		for _, key := range selectedProvider.RequiredEnv {
			if os.Getenv(key) == "" {
				missing = append(missing, selectedProvider.Name+":"+key)
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ","))
	}
	return nil
}

func runProviders(selected []provider) ([]string, []providerFailure) {
	succeeded := make([]bool, len(selected))
	failures := make([]error, len(selected))

	var wg sync.WaitGroup
	wg.Add(len(selected))
	for i := range selected {
		go func(index int) {
			defer wg.Done()
			if err := selected[index].Run(); err != nil {
				failures[index] = err
				return
			}
			succeeded[index] = true
		}(i)
	}
	wg.Wait()

	successNames := make([]string, 0, len(selected))
	failed := make([]providerFailure, 0)
	for i, selectedProvider := range selected {
		if succeeded[i] {
			successNames = append(successNames, selectedProvider.Name)
		} else {
			failed = append(failed, providerFailure{
				Name:  selectedProvider.Name,
				Error: failures[i].Error(),
			})
		}
	}
	return successNames, failed
}
