package engine

import (
	"errors"
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
