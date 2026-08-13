package ec2

import (
	"testing"
)

func TestFormatClockSpeedFromMhz(t *testing.T) {
	tests := []struct {
		speedMhz uint
		want     string
	}{
		{2500, "2.5 GHz"},
		{2700, "2.7 GHz"},
		{3100, "3.1 GHz"},
	}
	for _, tc := range tests {
		if got := formatClockSpeedFromMhz(tc.speedMhz); got != tc.want {
			t.Errorf("formatClockSpeedFromMhz(%d) = %q, want %q", tc.speedMhz, got, tc.want)
		}
	}
}

func TestAddExtraDetailsSetsClockSpeedFromMeasuredData(t *testing.T) {
	instance := &EC2Instance{InstanceType: "m6g.medium"}
	instance.addExtraDetails()

	if instance.ClockSpeedGhz == nil {
		t.Fatal("ClockSpeedGhz is nil, want 2.5 GHz from measured extras data")
	}
	if *instance.ClockSpeedGhz != "2.5 GHz" {
		t.Errorf("ClockSpeedGhz = %q, want %q", *instance.ClockSpeedGhz, "2.5 GHz")
	}
}

func TestApplyAPIInstanceDescription(t *testing.T) {
	cores := int32(2)
	maxENIs := int32(3)
	ipsPerENI := int32(10)
	baselineNetwork := 1.25
	peakNetwork := 12.5
	maxBandwidth := int32(10000)
	maxIOPS := int32(40000)
	maxThroughput := 1250.0
	diskCount := int32(1)
	diskSize := int64(118)
	fpgaCount := int32(2)
	bareMetal := false
	networkPerformance := "Up to 12.5 Gigabit"

	instance := &EC2Instance{InstanceType: "m7i.large"}
	applyAPIInstanceDescription(instance, &APIInstanceTypeInfo{
		BareMetal:            &bareMetal,
		Hypervisor:           "nitro",
		NitroEnclavesSupport: "supported",
		ProcessorInfo: &APIProcessorInfo{
			SupportedArchitectures: []string{"x86_64"},
		},
		VCpuInfo: &APIVCpuInfo{DefaultCores: &cores},
		NetworkInfo: &APINetworkInfo{
			NetworkPerformance:        &networkPerformance,
			EnaSupport:                "required",
			MaximumNetworkInterfaces:  &maxENIs,
			Ipv4AddressesPerInterface: &ipsPerENI,
			NetworkCards: []APINetworkCardInfo{{
				BaselineBandwidthInGbps: &baselineNetwork,
				PeakBandwidthInGbps:     &peakNetwork,
			}},
		},
		EbsInfo: &APIEbsInfo{
			EbsOptimizedInfo: &APIEbsOptimizedInfo{
				MaximumBandwidthInMbps:  &maxBandwidth,
				MaximumIops:             &maxIOPS,
				MaximumThroughputInMBps: &maxThroughput,
			},
		},
		InstanceStorageInfo: &APIInstanceStorageInfo{
			NvmeSupport: "required",
			Disks: []APIDiskInfo{{
				Count:    &diskCount,
				SizeInGB: &diskSize,
				Type:     "ssd",
			}},
		},
		FpgaInfo: &APIFpgaInfo{
			Fpgas: []APIFpgaDeviceInfo{{Count: &fpgaCount}},
		},
	})

	if len(instance.Arch) != 1 || instance.Arch[0] != "x86_64" {
		t.Fatalf("Arch = %v", instance.Arch)
	}
	if instance.Cores == nil || *instance.Cores != 2 {
		t.Fatalf("Cores = %v", instance.Cores)
	}
	if instance.VPC == nil || instance.VPC.MaxENIs != 3 || instance.VPC.IPsPerENI != 10 {
		t.Fatalf("VPC = %#v", instance.VPC)
	}
	if instance.BaselineBandwidth == nil || *instance.BaselineBandwidth != 1.25 ||
		instance.BurstBandwidth == nil || *instance.BurstBandwidth != 12.5 {
		t.Fatalf("network bandwidth = %v/%v", instance.BaselineBandwidth, instance.BurstBandwidth)
	}
	if !instance.EBSOptimized || instance.EBSIOPS != 40000 || instance.EBSMaxBandwidth != 10000 {
		t.Fatalf("EBS fields = %#v", instance)
	}
	if instance.Storage == nil || instance.Storage.Devices != 1 || instance.Storage.Size != 118 || !instance.Storage.NVMeSSD {
		t.Fatalf("Storage = %#v", instance.Storage)
	}
	if instance.FPGA != 2 {
		t.Fatalf("FPGA = %d", instance.FPGA)
	}
	if instance.NitroSupport == nil || !*instance.NitroSupport ||
		instance.NitroEnclaveSupport == nil || !*instance.NitroEnclaveSupport {
		t.Fatalf("Nitro fields = %v/%v", instance.NitroSupport, instance.NitroEnclaveSupport)
	}
}

func TestApplyAPIInstanceDescriptionAllowsMissingOptionalFields(t *testing.T) {
	instance := &EC2Instance{InstanceType: "m7i.large"}
	applyAPIInstanceDescription(instance, &APIInstanceTypeInfo{})

	if instance.VPC != nil || instance.Storage != nil || instance.Cores != nil {
		t.Fatalf("unexpected optional fields: VPC=%#v Storage=%#v Cores=%#v", instance.VPC, instance.Storage, instance.Cores)
	}
}
