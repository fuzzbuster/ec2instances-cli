package ec2

import (
	"log"
)

var OK_NVME_STRINGS = map[string]bool{
	"supported": true,
	"required":  true,
}

func addInstanceStorageDetails(instance *EC2Instance, apiDescription *APIInstanceTypeInfo) {
	storageInfo := apiDescription.InstanceStorageInfo
	if storageInfo != nil {
		if len(storageInfo.Disks) == 0 {
			log.Default().Println("No disks found for", instance.InstanceType)
			return
		}
		disk := storageInfo.Disks[0]
		if disk.Count == nil || disk.SizeInGB == nil {
			return
		}
		instance.Storage = &Storage{
			NVMeSSD: OK_NVME_STRINGS[string(storageInfo.NvmeSupport)],
			SSD:     disk.Type == "ssd",

			// Redundant column - but here for legacy reasons. Any SSD will have trim support
			TrimSupport: disk.Type == "ssd",

			// Always seems to be false in the Python code
			// TODO: add this? Unsure why its not in the Python code
			StorageNeedsInitalization: false,
			IncludesSwapPartition:     false,

			Devices:  int(*disk.Count),
			Size:     int(*disk.SizeInGB),
			SizeUnit: "GB",
		}
	}
}
