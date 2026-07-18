package main

// Static data mirroring the HWiNFO64 "System Summary" screenshot
// (AMD Ryzen 7 5800X / ASUS ROG STRIX B550-F GAMING WIFI II / RTX 3080).

// fflag is one CPU feature flag. state: 0 = unsupported (gray),
// 1 = supported (green), 2 = special (red).
type fflag struct {
	name  string
	state int
}

// features is the 7-column x 6-row CPU feature grid.
var features = [][]fflag{
	{{"MMX", 1}, {"3DNow!", 0}, {"3DNow!-2", 0}, {"SSE", 1}, {"SSE-2", 1}, {"SSE-3", 1}, {"SSSE-3", 1}},
	{{"SSE4A", 1}, {"SSE4.1", 1}, {"SSE4.2", 1}, {"AVX", 1}, {"AVX2", 1}, {"AVX-512", 0}, {"AVX10", 0}},
	{{"BMI2", 1}, {"ABM", 1}, {"TBM", 0}, {"FMA", 1}, {"ADX", 1}, {"XOP", 0}, {"AMX", 0}},
	{{"DEP", 1}, {"AMD-V", 2}, {"SMX", 0}, {"SMEP", 1}, {"SMAP", 1}, {"TSX", 0}, {"MPX", 0}},
	{{"EM64T", 1}, {"EIST", 0}, {"TM1", 0}, {"TM2", 0}, {"CPB", 1}, {"SST", 0}, {"SST", 0}},
	{{"AES-NI", 1}, {"RDRAND", 1}, {"RDSEED", 1}, {"SHA", 1}, {"SGX", 0}, {"SME", 0}, {"APX", 0}},
}

// opRows: Operating Point table (name, Clock, Ratio, Bus, VID).
var opRows = [][5]string{
	{"Minimum Clock", "550.0 MHz", "x5.50", "100.0 MHz", "-"},
	{"Base Clock", "3800.0 MHz", "x38.00", "100.0 MHz", "-"},
	{"Boost Max", "4850.0 MHz", "x48.50", "100.0 MHz", "-"},
	{"PBO Max", "4850.0 MHz", "x48.50", "100.0 MHz", "-"},
	{"Avg. Active Clock", "3911.7 MHz", "x39.19", "99.8 MHz", "1.4336 V"},
	{"Avg. Effective Clock", "381.6 MHz", "x3.82", "-", "-"},
}

// memHeaders / memRows: memory module timing table.
var memHeaders = []string{"Clock", "tCL", "tRCD", "tRP", "tRAS", "RC", "Ext.", "V"}

var memRows = [][8]string{
	{"1200", "16", "16", "16", "39", "56", "XMP", "1.20"},
	{"1067", "15", "15", "15", "35", "49", "XMP", "1.20"},
	{"933.3", "13", "13", "13", "31", "43", "XMP", "1.20"},
	{"800.0", "11", "11", "11", "26", "37", "XMP", "1.20"},
	{"666.7", "9", "9", "9", "22", "31", "XMP", "1.20"},
	{"1200", "16", "16", "16", "39", "56", "-", "1.20"},
	{"1067", "15", "15", "15", "35", "49", "-", "1.20"},
	{"933.3", "13", "13", "13", "31", "43", "-", "1.20"},
	{"800.0", "11", "11", "11", "26", "37", "-", "1.20"},
	{"666.7", "9", "9", "9", "22", "31", "-", "1.20"},
}

// drive is one storage device row.
type drive struct {
	ok    bool // green check when true
	iface string
	model string
}

var drives = []drive{
	{true, "SATA 6 Gb/s @ 6Gb/s", "CT1000MX500SSD1 [1 TB]"},
	{true, "NVMe x4 8.0 GT/s", "WDS100T2X0C-00L350 [1 TB]"},
	{true, "SATA 6 Gb/s @ 3Gb/s", "WDC WD40EZRX-00SPEB0 [4 TB]"},
	{false, "ATAPI", "PIONEER BD-RW   BDR-206D [BD-RE]"},
}
