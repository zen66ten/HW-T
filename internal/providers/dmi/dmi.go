// Package dmi parses the SMBIOS/DMI table directly from
// /sys/firmware/dmi/tables/DMI (no dmidecode dependency) and exposes the
// platform inventory: BIOS, system, board, processors, memory devices.
//
// The table is a sequence of structures: a 4-byte header (type, length,
// handle), a formatted area of length-4 bytes, then a string-set of
// NUL-terminated strings ended by an extra NUL. String fields in the
// formatted area are 1-based indexes into that set; 0 means "none".
package dmi

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// Info is the decoded subset of SMBIOS structure types HW-T uses.
type Info struct {
	BIOS       BIOS
	System     System
	Board      Board
	Processors []Processor
	Memory     []MemoryDevice
}

type BIOS struct {
	Vendor, Version, Date string
}

type System struct {
	Manufacturer, Product, Version, Serial, UUID, Family string
}

type Board struct {
	Manufacturer, Product, Version, Serial string
}

type Processor struct {
	Socket, Manufacturer, Version string
	MaxSpeedMHz, CurSpeedMHz      int
	Cores, Threads                int
}

type MemoryDevice struct {
	Locator, BankLocator     string
	SizeMB                   int
	Type                     string
	SpeedMTs, ConfiguredMTs  int
	Manufacturer, PartNumber string
}

// memoryTypes maps SMBIOS type 17 Memory Type enum values (DSP0134 §7.18.2).
var memoryTypes = map[byte]string{
	0x01: "Other", 0x02: "Unknown", 0x0F: "SDRAM",
	0x12: "DDR", 0x13: "DDR2", 0x18: "DDR3", 0x1A: "DDR4",
	0x1E: "LPDDR4", 0x22: "DDR5", 0x23: "LPDDR5",
}

type structure struct {
	typ     byte
	handle  uint16
	fm      []byte // formatted area including the 4-byte header
	strings []string
}

// str returns the string referenced by the 1-based index at fm[offset],
// or "" when the field is absent, out of range, or unset.
func (s structure) str(offset int) string {
	if offset >= len(s.fm) {
		return ""
	}
	idx := int(s.fm[offset])
	if idx == 0 || idx > len(s.strings) {
		return ""
	}
	return strings.TrimSpace(s.strings[idx-1])
}

func (s structure) byteAt(offset int) byte {
	if offset >= len(s.fm) {
		return 0
	}
	return s.fm[offset]
}

func (s structure) wordAt(offset int) uint16 {
	if offset+2 > len(s.fm) {
		return 0
	}
	return binary.LittleEndian.Uint16(s.fm[offset:])
}

func (s structure) dwordAt(offset int) uint32 {
	if offset+4 > len(s.fm) {
		return 0
	}
	return binary.LittleEndian.Uint32(s.fm[offset:])
}

// Parse decodes a raw DMI table blob.
func Parse(table []byte) (*Info, error) {
	structs, err := split(table)
	if err != nil {
		return nil, err
	}

	info := &Info{}
	for _, s := range structs {
		switch s.typ {
		case 0:
			info.BIOS = BIOS{
				Vendor:  s.str(0x04),
				Version: s.str(0x05),
				Date:    s.str(0x08),
			}
		case 1:
			info.System = System{
				Manufacturer: s.str(0x04),
				Product:      s.str(0x05),
				Version:      s.str(0x06),
				Serial:       s.str(0x07),
				UUID:         formatUUID(s.fm, 0x08),
				Family:       s.str(0x1A),
			}
		case 2:
			info.Board = Board{
				Manufacturer: s.str(0x04),
				Product:      s.str(0x05),
				Version:      s.str(0x06),
				Serial:       s.str(0x07),
			}
		case 4:
			p := Processor{
				Socket:       s.str(0x04),
				Manufacturer: s.str(0x07),
				Version:      s.str(0x10),
				MaxSpeedMHz:  int(s.wordAt(0x14)),
				CurSpeedMHz:  int(s.wordAt(0x16)),
				Cores:        int(s.byteAt(0x23)),
				Threads:      int(s.byteAt(0x25)),
			}
			// 0xFF in the byte fields means "use the SMBIOS 3.0 word fields".
			if p.Cores == 0xFF {
				p.Cores = int(s.wordAt(0x2A))
			}
			if p.Threads == 0xFF {
				p.Threads = int(s.wordAt(0x2E))
			}
			info.Processors = append(info.Processors, p)
		case 17:
			m := MemoryDevice{
				Locator:       s.str(0x10),
				BankLocator:   s.str(0x11),
				Type:          memoryTypes[s.byteAt(0x12)],
				SizeMB:        memorySizeMB(s),
				SpeedMTs:      int(s.wordAt(0x15)),
				ConfiguredMTs: int(s.wordAt(0x20)),
				Manufacturer:  s.str(0x17),
				PartNumber:    s.str(0x1A),
			}
			if m.Type == "" {
				m.Type = "type " + strconv.Itoa(int(s.byteAt(0x12)))
			}
			info.Memory = append(info.Memory, m)
		}
	}
	return info, nil
}

// memorySizeMB decodes type 17 Size (word at 0x0C): 0 = empty slot,
// 0xFFFF = unknown, bit 15 set = KB units, 0x7FFF = use extended dword.
func memorySizeMB(s structure) int {
	size := s.wordAt(0x0C)
	switch size {
	case 0, 0xFFFF:
		return 0
	case 0x7FFF:
		return int(s.dwordAt(0x1C) &^ (1 << 31))
	}
	if size&0x8000 != 0 {
		return int(size&0x7FFF) / 1024
	}
	return int(size)
}

func formatUUID(fm []byte, offset int) string {
	if offset+16 > len(fm) {
		return ""
	}
	u := fm[offset : offset+16]
	allZero, allFF := true, true
	for _, b := range u {
		allZero = allZero && b == 0
		allFF = allFF && b == 0xFF
	}
	if allZero || allFF {
		return ""
	}
	// Per DSP0134 the first three fields are little-endian.
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%x",
		u[3], u[2], u[1], u[0], u[5], u[4], u[7], u[6], u[8], u[9], u[10:16])
}

// split walks the table into structures with their string sets. All offsets
// are bounds-checked: a truncated or corrupt table yields what was parsed
// before the damage, never a panic (parsers get fuzzed, see SPEC §12).
func split(table []byte) ([]structure, error) {
	var out []structure
	off := 0
	for off+4 <= len(table) {
		typ := table[off]
		length := int(table[off+1])
		handle := binary.LittleEndian.Uint16(table[off+2:])
		if length < 4 || off+length > len(table) {
			return out, fmt.Errorf("dmi: corrupt structure header at offset %d", off)
		}

		s := structure{typ: typ, handle: handle, fm: table[off : off+length]}

		// String set: NUL-terminated strings, ended by an extra NUL.
		p := off + length
		for p < len(table) {
			end := p
			for end < len(table) && table[end] != 0 {
				end++
			}
			if end == p {
				p++ // the extra NUL closing the set
				break
			}
			s.strings = append(s.strings, string(table[p:end]))
			p = end + 1
		}
		// A structure with no strings is terminated by two NULs.
		if p < len(table) && len(s.strings) == 0 && table[p] == 0 {
			p++
		}

		out = append(out, s)
		if typ == 127 { // end-of-table
			break
		}
		off = p
	}
	return out, nil
}
