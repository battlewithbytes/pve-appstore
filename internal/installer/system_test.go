package installer

import (
	"testing"
)

// =============================================================================
// extractTokenValue tests
// =============================================================================

func TestExtractTokenValue_ValidJSON(t *testing.T) {
	input := `{"full-tokenid":"appstore@pve!appstore","info":{"privsep":"0"},"value":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}`
	got := extractTokenValue(input)
	if got != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Fatalf("expected UUID, got %q", got)
	}
}

func TestExtractTokenValue_JSONWithWhitespace(t *testing.T) {
	input := `  {"full-tokenid":"appstore@pve!appstore","value":"12345678-1234-1234-1234-123456789abc"}  `
	got := extractTokenValue(input)
	if got != "12345678-1234-1234-1234-123456789abc" {
		t.Fatalf("expected UUID, got %q", got)
	}
}

func TestExtractTokenValue_FallbackUUID(t *testing.T) {
	// If JSON parsing fails, try to find a UUID-like line
	input := "some header output\n12345678-1234-1234-1234-123456789abc\nmore output"
	got := extractTokenValue(input)
	if got != "12345678-1234-1234-1234-123456789abc" {
		t.Fatalf("expected UUID from fallback, got %q", got)
	}
}

func TestExtractTokenValue_PlainText(t *testing.T) {
	// Last resort: return trimmed input
	input := "  sometoken  "
	got := extractTokenValue(input)
	if got != "sometoken" {
		t.Fatalf("expected trimmed token, got %q", got)
	}
}

func TestExtractTokenValue_EmptyJSON(t *testing.T) {
	input := `{"value":""}`
	got := extractTokenValue(input)
	// Empty value triggers fallback to scan or trimmed raw
	if got == "" {
		t.Fatal("expected non-empty result")
	}
}

// =============================================================================
// ParseNumerics tests
// =============================================================================

func TestParseNumerics_Valid(t *testing.T) {
	a := &InstallerAnswers{
		CoresStr:    "4",
		MemoryMBStr: "2048",
		DiskGBStr:   "20",
		PortStr:     "8080",
	}
	parsed, err := a.ParseNumerics()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Cores != 4 {
		t.Errorf("expected cores=4, got %d", parsed.Cores)
	}
	if parsed.MemoryMB != 2048 {
		t.Errorf("expected memory=2048, got %d", parsed.MemoryMB)
	}
	if parsed.DiskGB != 20 {
		t.Errorf("expected disk=20, got %d", parsed.DiskGB)
	}
	if parsed.Port != 8080 {
		t.Errorf("expected port=8080, got %d", parsed.Port)
	}
}

func TestParseNumerics_InvalidCores(t *testing.T) {
	a := &InstallerAnswers{
		CoresStr:    "0",
		MemoryMBStr: "512",
		DiskGBStr:   "10",
		PortStr:     "8080",
	}
	_, err := a.ParseNumerics()
	if err == nil {
		t.Fatal("expected error for zero cores")
	}
}

func TestParseNumerics_NegativeCores(t *testing.T) {
	a := &InstallerAnswers{
		CoresStr:    "-1",
		MemoryMBStr: "512",
		DiskGBStr:   "10",
		PortStr:     "8080",
	}
	_, err := a.ParseNumerics()
	if err == nil {
		t.Fatal("expected error for negative cores")
	}
}

func TestParseNumerics_NonNumericCores(t *testing.T) {
	a := &InstallerAnswers{
		CoresStr:    "abc",
		MemoryMBStr: "512",
		DiskGBStr:   "10",
		PortStr:     "8080",
	}
	_, err := a.ParseNumerics()
	if err == nil {
		t.Fatal("expected error for non-numeric cores")
	}
}

func TestParseNumerics_MemoryTooLow(t *testing.T) {
	a := &InstallerAnswers{
		CoresStr:    "2",
		MemoryMBStr: "64",
		DiskGBStr:   "10",
		PortStr:     "8080",
	}
	_, err := a.ParseNumerics()
	if err == nil {
		t.Fatal("expected error for memory < 128")
	}
}

func TestParseNumerics_DiskZero(t *testing.T) {
	a := &InstallerAnswers{
		CoresStr:    "2",
		MemoryMBStr: "512",
		DiskGBStr:   "0",
		PortStr:     "8080",
	}
	_, err := a.ParseNumerics()
	if err == nil {
		t.Fatal("expected error for zero disk")
	}
}

func TestParseNumerics_PortOutOfRange(t *testing.T) {
	a := &InstallerAnswers{
		CoresStr:    "2",
		MemoryMBStr: "512",
		DiskGBStr:   "10",
		PortStr:     "99999",
	}
	_, err := a.ParseNumerics()
	if err == nil {
		t.Fatal("expected error for port > 65535")
	}
}

func TestParseNumerics_PortZero(t *testing.T) {
	a := &InstallerAnswers{
		CoresStr:    "2",
		MemoryMBStr: "512",
		DiskGBStr:   "10",
		PortStr:     "0",
	}
	_, err := a.ParseNumerics()
	if err == nil {
		t.Fatal("expected error for port 0")
	}
}

func TestParseNumerics_WhitespaceHandling(t *testing.T) {
	a := &InstallerAnswers{
		CoresStr:    " 4 ",
		MemoryMBStr: " 2048 ",
		DiskGBStr:   " 20 ",
		PortStr:     " 8080 ",
	}
	parsed, err := a.ParseNumerics()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Cores != 4 || parsed.MemoryMB != 2048 || parsed.DiskGB != 20 || parsed.Port != 8080 {
		t.Error("whitespace was not trimmed correctly")
	}
}

// =============================================================================
// EffectivePool tests
// =============================================================================

func TestEffectivePool_Existing(t *testing.T) {
	a := &InstallerAnswers{PoolChoice: "mypool"}
	if a.EffectivePool() != "mypool" {
		t.Errorf("expected 'mypool', got %q", a.EffectivePool())
	}
}

func TestEffectivePool_NewPool(t *testing.T) {
	a := &InstallerAnswers{PoolChoice: "__new__", NewPool: "appstore"}
	if a.EffectivePool() != "appstore" {
		t.Errorf("expected 'appstore', got %q", a.EffectivePool())
	}
}

// =============================================================================
// ValidatePositiveInt tests
// =============================================================================

func TestValidatePositiveInt_Valid(t *testing.T) {
	for _, v := range []string{"1", "100", "999999"} {
		if err := ValidatePositiveInt(v); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", v, err)
		}
	}
}

func TestValidatePositiveInt_Invalid(t *testing.T) {
	for _, v := range []string{"0", "-1", "abc", "", " "} {
		if err := ValidatePositiveInt(v); err == nil {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

// =============================================================================
// ValidatePort tests
// =============================================================================

func TestValidatePort_Valid(t *testing.T) {
	for _, v := range []string{"1", "80", "443", "8080", "65535"} {
		if err := ValidatePort(v); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", v, err)
		}
	}
}

func TestValidatePort_Invalid(t *testing.T) {
	for _, v := range []string{"0", "-1", "65536", "abc", ""} {
		if err := ValidatePort(v); err == nil {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

// =============================================================================
// ValidateMemory tests
// =============================================================================

func TestValidateMemory_Valid(t *testing.T) {
	for _, v := range []string{"128", "256", "512", "1024", "65536"} {
		if err := ValidateMemory(v); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", v, err)
		}
	}
}

func TestValidateMemory_Invalid(t *testing.T) {
	for _, v := range []string{"0", "64", "127", "-1", "abc", ""} {
		if err := ValidateMemory(v); err == nil {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

// =============================================================================
// Storage discovery parsing tests
// =============================================================================

func TestDiscoverStorages_FiltersByContent(t *testing.T) {
	// We can't easily test discoverStorages() directly because it calls exec.Command.
	// Instead, test the filtering logic by extracting what it does:
	// It filters storages where Content contains "rootdir" or "images".
	type storage struct {
		ID      string
		Type    string
		Content string
	}

	all := []storage{
		{"local-lvm", "lvmthin", "rootdir,images"},
		{"local", "dir", "iso,vztmpl,backup"},
		{"zfs-pool", "zfspool", "rootdir,images"},
		{"nfs-share", "nfs", "backup"},
	}

	// Replicate filter logic
	var filtered []StorageInfo
	for _, s := range all {
		if contains(s.Content, "rootdir") || contains(s.Content, "images") {
			filtered = append(filtered, StorageInfo{
				ID:      s.ID,
				Type:    s.Type,
				Content: s.Content,
			})
		}
	}

	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered storages, got %d", len(filtered))
	}
	if filtered[0].ID != "local-lvm" {
		t.Errorf("expected first storage to be 'local-lvm', got %q", filtered[0].ID)
	}
	if filtered[1].ID != "zfs-pool" {
		t.Errorf("expected second storage to be 'zfs-pool', got %q", filtered[1].ID)
	}
}

// contains is a test helper that checks if a comma-separated string contains a value.
func contains(csv, val string) bool {
	for _, part := range splitCSV(csv) {
		if part == val {
			return true
		}
	}
	return false
}

func splitCSV(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

// =============================================================================
// Bridge discovery parsing tests
// =============================================================================

func TestParseBridgeOutput(t *testing.T) {
	// Replicate the bridge parsing logic from discoverBridges
	output := `lo               UNKNOWN        00:00:00:00:00:00 <LOOPBACK,UP,LOWER_UP>
eth0             UP             aa:bb:cc:dd:ee:ff <BROADCAST,MULTICAST,UP,LOWER_UP>
vmbr0            UP             aa:bb:cc:dd:ee:00 <BROADCAST,MULTICAST,UP,LOWER_UP>
vmbr1            UP             aa:bb:cc:dd:ee:01 <BROADCAST,MULTICAST,UP,LOWER_UP>
docker0          DOWN           02:42:xx:xx:xx:xx <NO-CARRIER,BROADCAST,MULTICAST,UP>
`
	var bridges []string
	for _, line := range splitLines(output) {
		fields := splitFields(line)
		if len(fields) >= 1 && len(fields[0]) >= 4 && fields[0][:4] == "vmbr" {
			bridges = append(bridges, fields[0])
		}
	}

	if len(bridges) != 2 {
		t.Fatalf("expected 2 bridges, got %d: %v", len(bridges), bridges)
	}
	if bridges[0] != "vmbr0" {
		t.Errorf("expected first bridge 'vmbr0', got %q", bridges[0])
	}
	if bridges[1] != "vmbr1" {
		t.Errorf("expected second bridge 'vmbr1', got %q", bridges[1])
	}
}

// splitLines splits string by newlines (test helper).
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// splitFields splits a string by whitespace (test helper).
func splitFields(s string) []string {
	var fields []string
	inField := false
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			if inField {
				fields = append(fields, s[start:i])
				inField = false
			}
		} else {
			if !inField {
				start = i
				inField = true
			}
		}
	}
	if inField {
		fields = append(fields, s[start:])
	}
	return fields
}

// =============================================================================
// Pool command construction test
// =============================================================================

func TestCreatePool_SkipsExistingPool(t *testing.T) {
	answers := &InstallerAnswers{PoolChoice: "existingpool"}
	// createPool returns early when PoolChoice != "__new__"
	err := createPool(answers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// GPU discovery tests (using PCIGPUInfo structures)
// =============================================================================

func TestBuildPCIGPUMap_Nil(t *testing.T) {
	m := buildPCIGPUMap(nil)
	if m != nil {
		t.Fatal("expected nil map for nil input")
	}
}

func TestBuildPCIGPUMap_Empty(t *testing.T) {
	m := buildPCIGPUMap([]PCIGPUInfo{})
	if m != nil {
		t.Fatal("expected nil map for empty input")
	}
}

func TestBuildPCIGPUMap_Valid(t *testing.T) {
	devices := []PCIGPUInfo{
		{ID: "0000:61:00.0", Vendor: "0x10de", VendorName: "NVIDIA", DeviceName: "RTX 3060"},
		{ID: "0000:00:02.0", Vendor: "0x8086", VendorName: "Intel", DeviceName: "UHD 630"},
	}
	m := buildPCIGPUMap(devices)
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m))
	}
	dev, ok := m["0000:61:00.0"]
	if !ok {
		t.Fatal("expected device at 0000:61:00.0")
	}
	if dev.VendorName != "NVIDIA" {
		t.Errorf("expected NVIDIA, got %q", dev.VendorName)
	}
}

// =============================================================================
// PCI vendor lookup tests
// =============================================================================

func TestPCIVendorLookup(t *testing.T) {
	tests := []struct {
		vendor   string
		wantType string
		found    bool
	}{
		{"0x8086", "intel", true},
		{"0x1002", "amd", true},
		{"0x10de", "nvidia", true},
		{"0x1a03", "aspeed", true},
		{"0x9999", "", false},
	}
	for _, tt := range tests {
		v, ok := pciVendors[tt.vendor]
		if ok != tt.found {
			t.Errorf("vendor %s: found=%v, want found=%v", tt.vendor, ok, tt.found)
			continue
		}
		if ok && v.typ != tt.wantType {
			t.Errorf("vendor %s: got type %q, want %q", tt.vendor, v.typ, tt.wantType)
		}
	}
}

// =============================================================================
// Form validator tests
// =============================================================================

func TestValidateProxmoxID_Valid(t *testing.T) {
	valid := []string{"appstore", "my-pool", "pool_1", "test.pool"}
	for _, v := range valid {
		if err := ValidateProxmoxID(v); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", v, err)
		}
	}
}

func TestValidateProxmoxID_Invalid(t *testing.T) {
	invalid := []string{"", " ", "-leading-dash", "has space", "has@special"}
	for _, v := range invalid {
		if err := ValidateProxmoxID(v); err == nil {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

func TestValidateIPAddress_Form_Valid(t *testing.T) {
	valid := []string{"0.0.0.0", "192.168.1.1", "10.0.0.1", "255.255.255.255"}
	for _, v := range valid {
		if err := ValidateIPAddress(v); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", v, err)
		}
	}
}

func TestValidateIPAddress_Form_Invalid(t *testing.T) {
	invalid := []string{"", "abc", "999.999.999.999"}
	for _, v := range invalid {
		if err := ValidateIPAddress(v); err == nil {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

func TestValidateCatalogURL_Valid(t *testing.T) {
	valid := []string{
		"https://github.com/example/repo",
		"http://git.local/repo",
		"git@github.com:user/repo.git",
	}
	for _, v := range valid {
		if err := ValidateCatalogURL(v); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", v, err)
		}
	}
}

func TestValidateCatalogURL_Invalid(t *testing.T) {
	invalid := []string{"", " ", "-badstart", "ftp://example.com", "just-a-string"}
	for _, v := range invalid {
		if err := ValidateCatalogURL(v); err == nil {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

func TestValidateCatalogBranch_Valid(t *testing.T) {
	valid := []string{"main", "develop", "feature/my-branch", "release-1.0"}
	for _, v := range valid {
		if err := ValidateCatalogBranch(v); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", v, err)
		}
	}
}

func TestValidateCatalogBranch_Invalid(t *testing.T) {
	invalid := []string{"", "-badstart", "has space", "has@symbol"}
	for _, v := range invalid {
		if err := ValidateCatalogBranch(v); err == nil {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

func TestValidateMultiSelectProxmoxIDs_Valid(t *testing.T) {
	if err := ValidateMultiSelectProxmoxIDs([]string{"local-lvm", "zfs-pool"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateMultiSelectProxmoxIDs_Empty(t *testing.T) {
	err := ValidateMultiSelectProxmoxIDs(nil)
	if err == nil {
		t.Fatal("expected error for empty selection")
	}
}

func TestValidateMultiSelectProxmoxIDs_InvalidEntry(t *testing.T) {
	err := ValidateMultiSelectProxmoxIDs([]string{"valid", "-invalid"})
	if err == nil {
		t.Fatal("expected error for invalid entry")
	}
}
