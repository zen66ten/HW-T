package edid

import (
	"encoding/binary"
	"testing"
)

// buildEDID assembles a synthetic 128-byte base block for a 2560x1440@144
// 27" monitor named "TESTMON" made by "ABC".
func buildEDID() []byte {
	e := make([]byte, 128)
	copy(e, header)

	// Manufacturer "ABC": A=1, B=2, C=3 packed 5-bit big-endian.
	binary.BigEndian.PutUint16(e[8:10], 1<<10|2<<5|3)
	binary.LittleEndian.PutUint16(e[10:12], 0xBEEF) // product code
	binary.LittleEndian.PutUint32(e[12:16], 12345)  // serial
	e[16] = 12                                      // week
	e[17] = 33                                      // 2023
	e[20] = 0x80                                    // digital
	e[21], e[22] = 60, 34                           // cm

	// DTD 1 (preferred): 2560x1440@144 -> pixel clock ~592 MHz.
	// hblank 160, vblank 65: refresh = 592e6/((2560+160)*(1440+65)) ~ 144.6
	d := e[54:72]
	binary.LittleEndian.PutUint16(d[0:2], 59200) // x10 kHz
	d[2] = 2560 & 0xFF
	d[3] = 160 & 0xFF
	d[4] = byte(2560>>8)<<4 | byte(160>>8)
	d[5] = 1440 & 0xFF
	d[6] = 65 & 0xFF
	d[7] = byte(1440>>8)<<4 | byte(65>>8)

	// Descriptor 2: monitor name "TESTMON\n".
	d = e[72:90]
	d[3] = 0xFC
	copy(d[5:], "TESTMON\x0a     ")

	// Descriptor 3: serial string.
	d = e[90:108]
	d[3] = 0xFF
	copy(d[5:], "SN-0042\x0a      ")

	return e
}

func TestParse(t *testing.T) {
	info, err := Parse(buildEDID())
	if err != nil {
		t.Fatal(err)
	}
	if info.Manufacturer != "ABC" {
		t.Errorf("Manufacturer = %q", info.Manufacturer)
	}
	if info.ProductCode != 0xBEEF || info.Serial != 12345 {
		t.Errorf("product/serial = %04x/%d", info.ProductCode, info.Serial)
	}
	if info.Name != "TESTMON" || info.SerialString != "SN-0042" {
		t.Errorf("name/serialstr = %q/%q", info.Name, info.SerialString)
	}
	if info.Year != 2023 || info.Week != 12 || !info.Digital {
		t.Errorf("year/week/digital = %d/%d/%v", info.Year, info.Week, info.Digital)
	}
	if info.HActive != 2560 || info.VActive != 1440 {
		t.Errorf("mode = %dx%d", info.HActive, info.VActive)
	}
	if info.RefreshHz < 143 || info.RefreshHz > 146 {
		t.Errorf("refresh = %v, want ~144.6", info.RefreshHz)
	}
	if got := info.ModeString(); got != "2560x1440 @ 145 Hz" {
		t.Errorf("ModeString = %q", got)
	}
	if got := info.SizeString(); got != `60x34 cm (27.2")` {
		t.Errorf("SizeString = %q", got)
	}
}

func TestParseRejects(t *testing.T) {
	if _, err := Parse(nil); err == nil {
		t.Error("nil input did not error")
	}
	if _, err := Parse(make([]byte, 64)); err == nil {
		t.Error("short input did not error")
	}
	bad := buildEDID()
	bad[0] = 0x42
	if _, err := Parse(bad); err == nil {
		t.Error("bad header did not error")
	}
}

func FuzzParse(f *testing.F) {
	f.Add(buildEDID())
	f.Fuzz(func(t *testing.T, data []byte) {
		Parse(data)
	})
}
