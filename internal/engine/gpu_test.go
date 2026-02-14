package engine

import "testing"

func TestHasNvidiaDevicesTrue(t *testing.T) {
	devices := []DevicePassthrough{
		{Path: "/dev/dri/renderD128"},
		{Path: "/dev/nvidia0"},
	}
	if !hasNvidiaDevices(devices) {
		t.Error("hasNvidiaDevices = false, want true")
	}
}

func TestHasNvidiaDevicesFalse(t *testing.T) {
	devices := []DevicePassthrough{
		{Path: "/dev/dri/renderD128"},
		{Path: "/dev/dri/card0"},
	}
	if hasNvidiaDevices(devices) {
		t.Error("hasNvidiaDevices = true, want false")
	}
}

func TestHasNvidiaDevicesEmpty(t *testing.T) {
	if hasNvidiaDevices(nil) {
		t.Error("hasNvidiaDevices(nil) = true, want false")
	}
}
