package dmi

import (
	"encoding/binary"
	"testing"
)

// buildStructure assembles one SMBIOS structure: formatted area + strings.
func buildStructure(typ byte, formatted []byte, strs ...string) []byte {
	out := []byte{typ, byte(4 + len(formatted)), 0, 0}
	out = append(out, formatted...)
	if len(strs) == 0 {
		return append(out, 0, 0)
	}
	for _, s := range strs {
		out = append(out, s...)
		out = append(out, 0)
	}
	return append(out, 0)
}

func word(v uint16) []byte {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, v)
	return b
}

func testTable() []byte {
	var table []byte

	// Type 0: BIOS. Offsets 0x04 vendor, 0x05 version, 0x08 date.
	bios := make([]byte, 0x14)
	bios[0x00], bios[0x01], bios[0x04] = 1, 2, 3
	table = append(table, buildStructure(0, bios, "AwesomeBIOS Corp", "F.42", "01/15/2026")...)

	// Type 2: baseboard.
	board := make([]byte, 0x0C)
	board[0x00], board[0x01], board[0x02], board[0x03] = 1, 2, 3, 4
	table = append(table, buildStructure(2, board, "Gigabyte", "X570 AORUS", "1.0", "SN123")...)

	// Type 4: processor. String indexes at 0x04 socket, 0x07 manufacturer,
	// 0x10 version; words at 0x14/0x16 speeds; bytes 0x23/0x25 cores/threads.
	proc := make([]byte, 0x30)
	proc[0x00], proc[0x03], proc[0x0C] = 1, 2, 3
	copy(proc[0x10:], word(3400)) // 0x14 max speed
	copy(proc[0x12:], word(3400)) // 0x16 current speed
	proc[0x1F] = 12               // 0x23 cores
	proc[0x21] = 24               // 0x25 threads
	table = append(table, buildStructure(4, proc, "AM4", "AMD", "Ryzen 9 5900X")...)

	// Type 17: memory device. Word 0x0C size (MB), strings 0x10/0x11
	// locator/bank, byte 0x12 type, words 0x15/0x20 speeds, strings
	// 0x17/0x1A manufacturer/part.
	mem := make([]byte, 0x24)
	copy(mem[0x08:], word(16384)) // 0x0C size = 16 GB
	mem[0x0C], mem[0x0D] = 1, 2   // 0x10, 0x11 locators
	mem[0x0E] = 0x1A              // 0x12 DDR4
	copy(mem[0x11:], word(3200))  // 0x15 speed
	mem[0x13] = 3                 // 0x17 manufacturer
	mem[0x16] = 4                 // 0x1A part number
	copy(mem[0x1C:], word(3000))  // 0x20 configured speed
	table = append(table, buildStructure(17, mem, "DIMM_A1", "BANK 0", "Corsair", "CMK32GX4M2Z3600C18")...)

	// Empty slot: size 0 -> skipped by the provider, parsed here.
	empty := make([]byte, 0x24)
	empty[0x0C] = 1
	table = append(table, buildStructure(17, empty, "DIMM_B1")...)

	// Type 127: end-of-table.
	table = append(table, buildStructure(127, nil)...)
	return table
}

func TestParse(t *testing.T) {
	info, err := Parse(testTable())
	if err != nil {
		t.Fatal(err)
	}

	if info.BIOS.Vendor != "AwesomeBIOS Corp" || info.BIOS.Version != "F.42" || info.BIOS.Date != "01/15/2026" {
		t.Errorf("BIOS = %+v", info.BIOS)
	}
	if info.Board.Manufacturer != "Gigabyte" || info.Board.Product != "X570 AORUS" || info.Board.Serial != "SN123" {
		t.Errorf("Board = %+v", info.Board)
	}

	if len(info.Processors) != 1 {
		t.Fatalf("got %d processors, want 1", len(info.Processors))
	}
	p := info.Processors[0]
	if p.Socket != "AM4" || p.Version != "Ryzen 9 5900X" || p.MaxSpeedMHz != 3400 || p.Cores != 12 || p.Threads != 24 {
		t.Errorf("Processor = %+v", p)
	}

	if len(info.Memory) != 2 {
		t.Fatalf("got %d memory devices, want 2", len(info.Memory))
	}
	m := info.Memory[0]
	if m.Locator != "DIMM_A1" || m.SizeMB != 16384 || m.Type != "DDR4" ||
		m.SpeedMTs != 3200 || m.ConfiguredMTs != 3000 || m.PartNumber != "CMK32GX4M2Z3600C18" {
		t.Errorf("Memory[0] = %+v", m)
	}
	if info.Memory[1].SizeMB != 0 {
		t.Errorf("empty slot SizeMB = %d, want 0", info.Memory[1].SizeMB)
	}
}

func TestParseCorruptAndTruncated(t *testing.T) {
	// Errors are fine, panics are not (§12: parsers get fuzzed).
	cases := [][]byte{
		nil,
		{0},
		{0, 1, 0, 0},         // length < 4
		{0, 200, 0, 0, 1, 2}, // length beyond table
		testTable()[:17],     // truncated mid-structure
		testTable()[:40],     // truncated mid-strings
	}
	for _, c := range cases {
		Parse(c)
	}
}

func FuzzParse(f *testing.F) {
	f.Add(testTable())
	f.Fuzz(func(t *testing.T, data []byte) {
		Parse(data)
	})
}
