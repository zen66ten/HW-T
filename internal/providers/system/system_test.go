package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOSReleaseField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "os-release")
	os.WriteFile(path, []byte(`NAME="Fedora Linux"
VERSION="44 (KDE Plasma Desktop Edition)"
PRETTY_NAME="Fedora Linux 44 (KDE Plasma Desktop Edition)"
ID=fedora
`), 0o644)

	if got := osReleaseField(path, "PRETTY_NAME"); got != "Fedora Linux 44 (KDE Plasma Desktop Edition)" {
		t.Errorf("PRETTY_NAME = %q", got)
	}
	if got := osReleaseField(path, "ID"); got != "fedora" {
		t.Errorf("ID = %q", got)
	}
	if got := osReleaseField(path, "MISSING"); got != "" {
		t.Errorf("missing field = %q, want empty", got)
	}
	if got := osReleaseField("/nonexistent", "NAME"); got != "" {
		t.Errorf("missing file = %q, want empty", got)
	}
}

func TestMeminfoField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meminfo")
	os.WriteFile(path, []byte("MemTotal:       32791980 kB\nMemFree:         1000000 kB\n"), 0o644)

	if got := meminfoField(path, "MemTotal"); got != 32791980 {
		t.Errorf("MemTotal = %d", got)
	}
	if got := meminfoField(path, "SwapTotal"); got != 0 {
		t.Errorf("missing key = %d, want 0", got)
	}
}
