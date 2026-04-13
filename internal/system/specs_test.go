package system

import (
	"runtime"
	"testing"
)

func TestDetectReturnsNonZeroValues(t *testing.T) {
	specs, err := Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if specs.TotalRAMMB == 0 {
		t.Error("TotalRAMMB = 0, want > 0")
	}
	if specs.CPUCores == 0 {
		t.Error("CPUCores = 0, want > 0")
	}
}

func TestDetectCPUCoresMatchesRuntime(t *testing.T) {
	specs, err := Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if specs.CPUCores != runtime.NumCPU() {
		t.Errorf("CPUCores = %d, want %d (runtime.NumCPU)", specs.CPUCores, runtime.NumCPU())
	}
}

func TestDetectReturnsPlausibleRAM(t *testing.T) {
	specs, err := Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	// Any real machine should have at least 256 MB. Upper bound sanity:
	// no single server should report more than 4 TB.
	if specs.TotalRAMMB < 256 {
		t.Errorf("TotalRAMMB = %d, suspiciously low (< 256 MB)", specs.TotalRAMMB)
	}
	if specs.TotalRAMMB > 4*1024*1024 {
		t.Errorf("TotalRAMMB = %d, suspiciously high (> 4 TB)", specs.TotalRAMMB)
	}
}
