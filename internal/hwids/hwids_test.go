package hwids

import (
	"bufio"
	"strings"
	"testing"
)

const sample = `# comment
10de  NVIDIA Corporation
	2208  GA102 [GeForce RTX 3080 Ti]
	2216  GA102 [GeForce RTX 3080 Lite Hash Rate]
		1458 403b  Some subsystem name
1022  Advanced Micro Devices, Inc. [AMD]
	43eb  500 Series Chipset SATA Controller
C 03  Display controller
	00  VGA compatible controller
	02  3D controller
C 01  Mass storage controller
	06  SATA controller
		01  AHCI 1.0
AT  keyboard-ish trailing section from usb.ids
	00  should be ignored
`

func testDB(t *testing.T) *DB {
	t.Helper()
	db := &DB{vendors: map[uint16]*vendor{}, classes: map[byte]*class{}}
	db.parse(bufio.NewScanner(strings.NewReader(sample)))
	return db
}

func TestLookups(t *testing.T) {
	db := testDB(t)

	if got := db.Vendor(0x10de); got != "NVIDIA Corporation" {
		t.Errorf("Vendor = %q", got)
	}
	if got := db.Product(0x10de, 0x2208); got != "GA102 [GeForce RTX 3080 Ti]" {
		t.Errorf("Product = %q", got)
	}
	if got := db.Product(0x1022, 0x43eb); got != "500 Series Chipset SATA Controller" {
		t.Errorf("Product = %q", got)
	}
	if got := db.ClassName(0x030000); got != "VGA compatible controller" {
		t.Errorf("ClassName = %q", got)
	}
	if got := db.ClassName(0x010601); got != "SATA controller" {
		t.Errorf("ClassName(sub+progif) = %q", got)
	}
}

func TestFallbacks(t *testing.T) {
	db := testDB(t)

	if got := db.Vendor(0xdead); got != "vendor dead" {
		t.Errorf("unknown vendor = %q", got)
	}
	if got := db.Product(0x10de, 0xffff); got != "device 10de:ffff" {
		t.Errorf("unknown product = %q", got)
	}
	if got := db.ClassName(0xff0000); got != "class ff0000" {
		t.Errorf("unknown class = %q", got)
	}

	// Zero DB (no database file found at all) must still answer.
	empty := Load("/nonexistent/pci.ids")
	if got := empty.Vendor(0x10de); got != "vendor 10de" {
		t.Errorf("empty DB vendor = %q", got)
	}

	// The usb.ids trailing section must not leak into vendors.
	if _, ok := db.vendors[0x00]; ok {
		t.Error("trailing section parsed as vendor")
	}
}
