// Package system builds the operating-system and platform inventory HWiNFO
// shows in its OS panel: distribution, kernel, firmware mode, Secure Boot
// and TPM state, total memory, hostname and uptime. All unprivileged.
package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/zen66ten/HW-T/internal/core"
)

// Provider exposes one static inventory device describing the OS and
// platform.
type Provider struct {
	sysfs     string
	osRelease string
	meminfo   string
}

func New(sysfsRoot string) *Provider {
	return &Provider{
		sysfs:     sysfsRoot,
		osRelease: "/etc/os-release",
		meminfo:   "/proc/meminfo",
	}
}

func (p *Provider) Name() string                   { return "system" }
func (p *Provider) DefaultInterval() time.Duration { return 0 }

func (p *Provider) Collect(ctx context.Context, dev core.DeviceID) ([]core.Reading, error) {
	return nil, nil
}

func (p *Provider) Discover(ctx context.Context) ([]core.Device, error) {
	attrs := map[string]string{}

	if name := osReleaseField(p.osRelease, "PRETTY_NAME"); name != "" {
		attrs["os"] = name
	}
	attrs["kernel"] = unameRelease()
	if host, err := os.Hostname(); err == nil {
		attrs["hostname"] = host
	}
	if _, err := os.Stat(p.sysfs + "/firmware/efi"); err == nil {
		attrs["firmware"] = "UEFI"
	} else {
		attrs["firmware"] = "Legacy BIOS"
	}
	attrs["secure_boot"] = secureBootState(p.sysfs)
	if entries, err := os.ReadDir(p.sysfs + "/class/tpm"); err == nil && len(entries) > 0 {
		attrs["tpm"] = "present (" + entries[0].Name() + ")"
	} else {
		attrs["tpm"] = "not present"
	}
	if kb := meminfoField(p.meminfo, "MemTotal"); kb > 0 {
		attrs["memory_total"] = fmt.Sprintf("%.1f GB", float64(kb)/(1024*1024))
	}
	if up := uptime(); up != "" {
		attrs["uptime"] = up
	}

	return []core.Device{{
		ID:       core.DeviceID("system:host"),
		Provider: "system",
		Name:     "Operating System",
		Attrs:    attrs,
	}}, nil
}

// osReleaseField extracts a KEY="value" field from an os-release file.
func osReleaseField(path, key string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if ok && k == key {
			return strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	return ""
}

func meminfoField(path, key string) int64 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, key+":") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, _ := strconv.ParseInt(fields[1], 10, 64)
				return kb
			}
		}
	}
	return 0
}

func unameRelease() string {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return ""
	}
	return strings.TrimRight(string(u.Sysname[:]), "\x00") + " " +
		strings.TrimRight(string(u.Release[:]), "\x00")
}

// secureBootState reads the EFI SecureBoot variable; its 5th byte is the
// boolean state (the first four bytes are the variable attributes).
func secureBootState(sysfs string) string {
	matches, _ := filepath.Glob(sysfs + "/firmware/efi/efivars/SecureBoot-*")
	if len(matches) == 0 {
		return "unknown"
	}
	data, err := os.ReadFile(matches[0])
	if err != nil || len(data) < 5 {
		return "unknown"
	}
	if data[4] == 1 {
		return "enabled"
	}
	return "disabled"
}

func uptime() string {
	var si unix.Sysinfo_t
	if err := unix.Sysinfo(&si); err != nil {
		return ""
	}
	d := time.Duration(si.Uptime) * time.Second
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	return fmt.Sprintf("%dh %dm", hours, mins)
}
