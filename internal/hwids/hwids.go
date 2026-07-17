// Package hwids parses the hwdata ID databases (pci.ids, usb.ids) that map
// numeric vendor/device/class IDs to human names. The system copy is
// preferred (kept current by distro updates); when none exists every lookup
// falls back to the raw hex ID, so callers never special-case a missing
// database (§5.6).
package hwids

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type vendor struct {
	name    string
	devices map[uint16]string
}

type class struct {
	name       string
	subclasses map[byte]string
}

// DB answers name lookups. The zero value is usable and answers every
// lookup with formatted raw IDs.
type DB struct {
	vendors map[uint16]*vendor
	classes map[byte]*class
}

// Load reads the first existing path. A missing/corrupt database returns an
// empty (but usable) DB.
func Load(paths ...string) *DB {
	db := &DB{vendors: map[uint16]*vendor{}, classes: map[byte]*class{}}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		defer f.Close()
		db.parse(bufio.NewScanner(f))
		break
	}
	return db
}

func (db *DB) parse(sc *bufio.Scanner) {
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var curVendor *vendor
	var curClass *class
	mode := "" // "vendor" | "class" | "other"

	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		switch {
		case !strings.HasPrefix(line, "\t"):
			curVendor, curClass = nil, nil
			if id, name, ok := cutID(line, 4); ok {
				curVendor = &vendor{name: name, devices: map[uint16]string{}}
				db.vendors[uint16(id)] = curVendor
				mode = "vendor"
			} else if rest, found := strings.CutPrefix(line, "C "); found {
				if id, name, ok := cutID(rest, 2); ok {
					curClass = &class{name: name, subclasses: map[byte]string{}}
					db.classes[byte(id)] = curClass
					mode = "class"
				}
			} else {
				// usb.ids trailing sections (AT, HID, R, ...): ignore.
				mode = "other"
			}

		case strings.HasPrefix(line, "\t\t"):
			// subsystem / protocol level: not surfaced.

		default: // one tab
			body := line[1:]
			switch mode {
			case "vendor":
				if curVendor != nil {
					if id, name, ok := cutID(body, 4); ok {
						curVendor.devices[uint16(id)] = name
					}
				}
			case "class":
				if curClass != nil {
					if id, name, ok := cutID(body, 2); ok {
						curClass.subclasses[byte(id)] = name
					}
				}
			}
		}
	}
}

// cutID splits "<hex><whitespace><name>" where the ID has the given number
// of hex digits.
func cutID(s string, digits int) (uint64, string, bool) {
	if len(s) < digits+2 {
		return 0, "", false
	}
	id, err := strconv.ParseUint(s[:digits], 16, 32)
	if err != nil {
		return 0, "", false
	}
	name := strings.TrimSpace(s[digits:])
	if name == "" {
		return 0, "", false
	}
	return id, name, true
}

// Vendor returns the vendor name, or the formatted raw ID when unknown.
func (db *DB) Vendor(id uint16) string {
	if v, ok := db.vendors[id]; ok {
		return v.name
	}
	return fmt.Sprintf("vendor %04x", id)
}

// Product returns the device/product name, or the formatted raw ID pair.
func (db *DB) Product(vendorID, productID uint16) string {
	if v, ok := db.vendors[vendorID]; ok {
		if name, ok := v.devices[productID]; ok {
			return name
		}
	}
	return fmt.Sprintf("device %04x:%04x", vendorID, productID)
}

// ClassName decodes a 24-bit PCI class code (base<<16 | sub<<8 | progif),
// preferring the subclass name.
func (db *DB) ClassName(code uint32) string {
	base := byte(code >> 16)
	sub := byte(code >> 8)
	if c, ok := db.classes[base]; ok {
		if name, ok := c.subclasses[sub]; ok {
			return name
		}
		return c.name
	}
	return fmt.Sprintf("class %06x", code)
}
