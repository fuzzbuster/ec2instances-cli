package ec2

import (
	"fmt"
	"github.com/fuzzbuster/ec2instances-cli/utils"
	"log"
	"strconv"
	"strings"

	"github.com/anaskhan96/soup"
)

func float64Ptr(f float64) *float64 {
	return &f
}

func processT2Row(instance *EC2Instance, childText string) error {
	credsPerHourFloat, err := strconv.ParseFloat(childText, 64)
	if err != nil {
		return fmt.Errorf("parse T2 credits per hour %q: %w", childText, err)
	}
	instance.BasePerformance = float64Ptr(credsPerHourFloat / 60)
	instance.BurstMinutes = float64Ptr(credsPerHourFloat * 24 / float64(instance.VCPU.Value()))
	return nil
}

func getT2Html() (*soup.Root, error) {
	t2Url := "https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/burstable-credits-baseline-concepts.html"
	doc, err := utils.LoadHTML(t2Url)
	if err != nil {
		return nil, fmt.Errorf("load T2 credits HTML: %w", err)
	}
	return doc, nil
}

func addT2Credits(instances map[string]*EC2Instance, t2HtmlGetter func() (*soup.Root, error)) error {
	log.Default().Println("Adding T2 credits to EC2")

	doc, err := t2HtmlGetter()
	if err != nil {
		return err
	}
	tableContainers := doc.FindAll("div", "class", "table-contents")
	if len(tableContainers) < 2 {
		return fmt.Errorf("find T2 credits table containers")
	}
	if tableContainers[1].Error != nil {
		return fmt.Errorf("load T2 credits table container: %w", tableContainers[1].Error)
	}
	tables := tableContainers[1].Find("table")
	if tables.Error != nil {
		return fmt.Errorf("find T2 credits table: %w", tables.Error)
	}

	tbody := tables.Find("tbody")
	if tbody.Error != nil {
		return fmt.Errorf("find T2 credits tbody: %w", tbody.Error)
	}

	rows := tbody.FindAll("tr")
	if len(rows) == 0 {
		return fmt.Errorf("find T2 credits rows")
	}

	for _, row := range rows {
		children := row.FindAll("td")
		var firstNodeText string

		childrenHtml := make([]string, len(children))
		for i, child := range children {
			childrenHtml[i] = child.HTML()
		}

		if len(children) > 1 {
			firstNodeText = toText(children[0])
			instance := instances[firstNodeText]
			if instance == nil {
				if strings.Contains(firstNodeText, ".") {
					utils.SendWarning("T2 credits data has unknown instance type", firstNodeText)
				}
			} else {
				childText := toText(children[1])
				if childText == "" {
					utils.SendWarning("T2 credits data has empty row", firstNodeText)
				} else {
					if err := processT2Row(instance, childText); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
