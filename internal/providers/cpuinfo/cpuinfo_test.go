package cpuinfo

import "testing"

const sampleProc = `processor	: 0
vendor_id	: AuthenticAMD
cpu family	: 23
model		: 113
model name	: AMD Ryzen 9 3900X 12-Core Processor
stepping	: 0
cpu cores	: 12
flags		: fpu vme de pse mmx sse sse2 pni ssse3 sse4_1 sse4_2 avx avx2 aes fma sha_ni svm rdrand rdseed bmi1 bmi2 lm
processor	: 1
model name	: AMD Ryzen 9 3900X 12-Core Processor
flags		: fpu vme de pse mmx sse sse2 avx avx2 lm
`

func TestParseProcCPUInfo(t *testing.T) {
	info := ParseProcCPUInfo(sampleProc)
	if info.ModelName != "AMD Ryzen 9 3900X 12-Core Processor" {
		t.Errorf("ModelName = %q", info.ModelName)
	}
	if info.Vendor != "AuthenticAMD" || info.Family != 23 || info.Model != 113 || info.Stepping != 0 {
		t.Errorf("identity = %+v", info)
	}
	if info.Cores != 12 {
		t.Errorf("Cores = %d, want 12", info.Cores)
	}
	if info.Threads != 2 {
		t.Errorf("Threads = %d, want 2 (one per processor block)", info.Threads)
	}
	if !info.Flags["avx2"] || !info.Flags["sha_ni"] || info.Flags["avx512f"] {
		t.Errorf("flags parsed wrong: %v", info.Flags)
	}
}

func TestInstructions(t *testing.T) {
	info := ParseProcCPUInfo(sampleProc)
	got := info.Instructions()
	// Ordered per featureDisplay; AMD-V present (svm), VT-x absent (no vmx).
	want := "MMX, SSE, SSE2, SSE3, SSSE3, SSE4.1, SSE4.2, x86-64, AMD-V, AES, AVX, AVX2, FMA3, SHA, BMI1, BMI2, RDRAND, RDSEED"
	if got != want {
		t.Errorf("Instructions() =\n  %q\nwant\n  %q", got, want)
	}
}
