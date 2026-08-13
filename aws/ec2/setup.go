package ec2

import (
	"github.com/fuzzbuster/ec2instances-cli/aws/awsutils"
	"github.com/fuzzbuster/ec2instances-cli/utils"
)

// Setup sets up the EC2 data processing module
func Setup(
	fg *utils.FunctionGroup,
	ec2ApiResponses *utils.SlowBuildingMap[string, *APIInstanceTypeInfo],
) (chan awsutils.RawRegion, chan awsutils.RawRegion) {
	// Start all the data getters in the background
	t2HtmlGetter := utils.BlockUntilDone(getT2Html)

	// Spawn the EC2 data processing threads
	ec2GlobalChannel := make(chan awsutils.RawRegion)
	ec2ChinaChannel := make(chan awsutils.RawRegion)
	getters := ec2DataGetters{
		t2HtmlGetter: t2HtmlGetter,
	}
	fg.Add(func() error {
		return processEC2Data(ec2ChinaChannel, ec2ApiResponses, true, getters)
	})
	fg.Add(func() error {
		return processEC2Data(ec2GlobalChannel, ec2ApiResponses, false, getters)
	})

	return ec2GlobalChannel, ec2ChinaChannel
}
