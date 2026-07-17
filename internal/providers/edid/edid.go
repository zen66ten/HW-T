// Package edid parses monitor EDID blobs exposed by DRM connectors
// (/sys/class/drm/card*-*/edid) into vendor, model, serial, native mode and
// physical size (§5.6). Own parser over the 128-byte base block; bounds-
// checked throughout (parsers get fuzzed, §12).
package edid

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// Info is the decoded subset HW-T surfaces.
type Info struct {
	Manufacturer string // PNP ID, e.g. "DEL", "GSM"
	ProductCode  uint16
	Serial       uint32
	SerialString string
	Name         string // monitor name descriptor
	Week, Year   int
	Digital      bool
	WidthCm      int
	HeightCm     int
	// Preferred (native) mode from the first detailed timing descriptor.
	PixelClockMHz float64
	HActive       int
	VActive       int
	RefreshHz     float64
}

var header = []byte{0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00}

// Parse decodes the 128-byte base EDID block (extension blocks are
// ignored). It errors on anything shorter or without the EDID header.
func Parse(raw []byte) (*Info, error) {
	if len(raw) < 128 {
		return nil, fmt.Errorf("edid: %d bytes, need 128", len(raw))
	}
	for i, b := range header {
		if raw[i] != b {
			return nil, fmt.Errorf("edid: bad header")
		}
	}

	info := &Info{
		ProductCode: binary.LittleEndian.Uint16(raw[10:12]),
		Serial:      binary.LittleEndian.Uint32(raw[12:16]),
		Week:        int(raw[16]),
		Year:        1990 + int(raw[17]),
		Digital:     raw[20]&0x80 != 0,
		WidthCm:     int(raw[21]),
		HeightCm:    int(raw[22]),
	}

	// Manufacturer: three 5-bit letters packed big-endian into bytes 8-9.
	m := binary.BigEndian.Uint16(raw[8:10])
	letters := []byte{
		byte(m>>10&0x1F) + 'A' - 1,
		byte(m>>5&0x1F) + 'A' - 1,
		byte(m&0x1F) + 'A' - 1,
	}
	if letters[0] >= 'A' && letters[0] <= 'Z' {
		info.Manufacturer = string(letters)
	}

	// Four 18-byte descriptor blocks at 54..125.
	for i := 0; i < 4; i++ {
		d := raw[54+i*18 : 54+(i+1)*18]
		if d[0] == 0 && d[1] == 0 {
			switch d[3] { // display descriptor
			case 0xFC:
				info.Name = descriptorText(d)
			case 0xFF:
				info.SerialString = descriptorText(d)
			}
			continue
		}
		if info.HActive == 0 { // first DTD = preferred mode
			decodeDTD(d, info)
		}
	}
	return info, nil
}

func decodeDTD(d []byte, info *Info) {
	pixClk := float64(binary.LittleEndian.Uint16(d[0:2])) / 100 // MHz
	hActive := int(d[2]) | int(d[4]>>4)<<8
	hBlank := int(d[3]) | int(d[4]&0x0F)<<8
	vActive := int(d[5]) | int(d[7]>>4)<<8
	vBlank := int(d[6]) | int(d[7]&0x0F)<<8

	info.PixelClockMHz = pixClk
	info.HActive = hActive
	info.VActive = vActive
	if total := float64((hActive + hBlank) * (vActive + vBlank)); total > 0 {
		info.RefreshHz = pixClk * 1e6 / total
	}
}

// descriptorText extracts the 13-char text payload (bytes 5-17), terminated
// by 0x0A per spec.
func descriptorText(d []byte) string {
	text := string(d[5:18])
	if i := strings.IndexByte(text, 0x0A); i >= 0 {
		text = text[:i]
	}
	return strings.TrimSpace(text)
}

// ModeString renders the native mode like "2560x1440 @ 144 Hz".
func (i *Info) ModeString() string {
	if i.HActive == 0 {
		return ""
	}
	if i.RefreshHz > 0 {
		return fmt.Sprintf("%dx%d @ %.0f Hz", i.HActive, i.VActive, i.RefreshHz)
	}
	return fmt.Sprintf("%dx%d", i.HActive, i.VActive)
}

// SizeString renders physical size like "60x34 cm (27.2\")".
func (i *Info) SizeString() string {
	if i.WidthCm == 0 || i.HeightCm == 0 {
		return ""
	}
	diagIn := math.Hypot(float64(i.WidthCm), float64(i.HeightCm)) / 2.54
	return fmt.Sprintf("%dx%d cm (%.1f\")", i.WidthCm, i.HeightCm, diagIn)
}
