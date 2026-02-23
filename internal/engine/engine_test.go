package engine

import (
	"errors"
	"runtime"
	"testing"
)

// --- extractHWAddr ---

func TestExtractHWAddrValid(t *testing.T) {
	net0 := "name=eth0,bridge=vmbr0,hwaddr=BC:24:11:AB:CD:EF,ip=dhcp"
	got := extractHWAddr(net0)
	want := "BC:24:11:AB:CD:EF"
	if got != want {
		t.Errorf("extractHWAddr = %q, want %q", got, want)
	}
}

func TestExtractHWAddrMissing(t *testing.T) {
	net0 := "name=eth0,bridge=vmbr0,ip=dhcp"
	got := extractHWAddr(net0)
	if got != "" {
		t.Errorf("extractHWAddr = %q, want empty", got)
	}
}

func TestExtractHWAddrMalformed(t *testing.T) {
	net0 := "name=eth0,hwaddr=NOTAMAC,ip=dhcp"
	got := extractHWAddr(net0)
	if got != "" {
		t.Errorf("extractHWAddr(malformed) = %q, want empty", got)
	}
}

func TestExtractHWAddrEmpty(t *testing.T) {
	got := extractHWAddr("")
	if got != "" {
		t.Errorf("extractHWAddr(\"\") = %q, want empty", got)
	}
}

func TestExtractHWAddrLowercase(t *testing.T) {
	net0 := "name=eth0,hwaddr=bc:24:11:ab:cd:ef"
	got := extractHWAddr(net0)
	want := "bc:24:11:ab:cd:ef"
	if got != want {
		t.Errorf("extractHWAddr(lowercase) = %q, want %q", got, want)
	}
}

// --- isContainerGone ---

func TestIsContainerGoneTrue(t *testing.T) {
	tests := []error{
		errors.New("container does not exist"),
		errors.New("no such container on this node"),
		errors.New("CT 100 not found"),
	}
	for _, err := range tests {
		if !isContainerGone(err) {
			t.Errorf("isContainerGone(%q) = false, want true", err)
		}
	}
}

func TestIsContainerGoneFalse(t *testing.T) {
	tests := []error{
		errors.New("connection refused"),
		errors.New("timeout"),
		errors.New("permission denied"),
	}
	for _, err := range tests {
		if isContainerGone(err) {
			t.Errorf("isContainerGone(%q) = true, want false", err)
		}
	}
}

func TestIsContainerGoneNil(t *testing.T) {
	if isContainerGone(nil) {
		t.Error("isContainerGone(nil) = true, want false")
	}
}

// --- ValidateCPUPin ---

func TestValidateCPUPinEmpty(t *testing.T) {
	if err := ValidateCPUPin(""); err != nil {
		t.Errorf("ValidateCPUPin(\"\") = %v, want nil", err)
	}
}

func TestValidateCPUPinSingle(t *testing.T) {
	if err := ValidateCPUPin("0"); err != nil {
		t.Errorf("ValidateCPUPin(\"0\") = %v, want nil", err)
	}
}

func TestValidateCPUPinList(t *testing.T) {
	if err := ValidateCPUPin("0,2,4"); err != nil {
		t.Errorf("ValidateCPUPin(\"0,2,4\") = %v, want nil", err)
	}
}

func TestValidateCPUPinRange(t *testing.T) {
	if err := ValidateCPUPin("0-3"); err != nil {
		t.Errorf("ValidateCPUPin(\"0-3\") = %v, want nil", err)
	}
}

func TestValidateCPUPinMixed(t *testing.T) {
	// Mix of range and individual — valid only if all within host CPU count
	maxCPU := runtime.NumCPU() - 1
	if maxCPU >= 7 {
		if err := ValidateCPUPin("0-3,6,7"); err != nil {
			t.Errorf("ValidateCPUPin(\"0-3,6,7\") = %v, want nil", err)
		}
	}
}

func TestValidateCPUPinDescendingRange(t *testing.T) {
	err := ValidateCPUPin("3-0")
	if err == nil {
		t.Error("ValidateCPUPin(\"3-0\") = nil, want error for descending range")
	}
}

func TestValidateCPUPinNegative(t *testing.T) {
	err := ValidateCPUPin("-1")
	if err == nil {
		t.Error("ValidateCPUPin(\"-1\") = nil, want error")
	}
}

func TestValidateCPUPinExceedsHost(t *testing.T) {
	// Use a CPU ID well beyond any real host
	err := ValidateCPUPin("99999")
	if err == nil {
		t.Error("ValidateCPUPin(\"99999\") = nil, want error for exceeding host CPU count")
	}
}

func TestValidateCPUPinEmptyToken(t *testing.T) {
	err := ValidateCPUPin("0,,2")
	if err == nil {
		t.Error("ValidateCPUPin(\"0,,2\") = nil, want error for empty token")
	}
}

func TestValidateCPUPinInvalidChars(t *testing.T) {
	err := ValidateCPUPin("abc")
	if err == nil {
		t.Error("ValidateCPUPin(\"abc\") = nil, want error")
	}
}

// --- CountPinnedCPUs ---

func TestCountPinnedCPUsEmpty(t *testing.T) {
	if got := CountPinnedCPUs(""); got != 0 {
		t.Errorf("CountPinnedCPUs(\"\") = %d, want 0", got)
	}
}

func TestCountPinnedCPUsSingle(t *testing.T) {
	if got := CountPinnedCPUs("0"); got != 1 {
		t.Errorf("CountPinnedCPUs(\"0\") = %d, want 1", got)
	}
}

func TestCountPinnedCPUsList(t *testing.T) {
	if got := CountPinnedCPUs("0,2,4"); got != 3 {
		t.Errorf("CountPinnedCPUs(\"0,2,4\") = %d, want 3", got)
	}
}

func TestCountPinnedCPUsRange(t *testing.T) {
	if got := CountPinnedCPUs("0-3"); got != 4 {
		t.Errorf("CountPinnedCPUs(\"0-3\") = %d, want 4", got)
	}
}

func TestCountPinnedCPUsMixed(t *testing.T) {
	// 0-3 = 4, plus 8-11 = 4 → total 8
	if got := CountPinnedCPUs("0-3,8-11"); got != 8 {
		t.Errorf("CountPinnedCPUs(\"0-3,8-11\") = %d, want 8", got)
	}
}

func TestCountPinnedCPUsSingleRange(t *testing.T) {
	// 5-5 = 1 CPU
	if got := CountPinnedCPUs("5-5"); got != 1 {
		t.Errorf("CountPinnedCPUs(\"5-5\") = %d, want 1", got)
	}
}
