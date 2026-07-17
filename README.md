# HW-T

HWiNFO64-like hardware inventory and sensor monitoring suite for Linux.

![hwt-gui sensors panel](docs/Untitled(1).jpg)

## Overview

HW-T is a hardware monitoring combo tool for Linux that combines hardware
inventory with real-time sensor readings. I built it because the lack of a
HWiNFO64 equivalent on Linux always annoyed me — the pieces are scattered
across a dozen utilities, and no single tool covers all of its functions.
It aggregates data from kernel interfaces and vendor tools: hwmon, cpufreq,
effective per-core clocks (APERF/MPERF via perf), RAPL package/core power,
NVIDIA GPUs (nvidia-smi), AMD GPUs (DRM sysfs), drive SMART health
(smartctl), DMI/SMBIOS — and serves it through a TUI, a one-shot CLI, a
Unix-socket API, and Prometheus metrics, with CSV/NDJSON logging and
threshold alerts.

**Components:**
- `hwtd` — monitoring daemon: discovers hardware, polls sensors in
  fast/medium/slow groups, tracks min/max/avg and history, evaluates
  alerts, drives the sensor log, serves all APIs
- `hwt-gui` — native desktop app (GPU-Z-style sensor rows with rolling
  bar graphs, click-to-graph history, inventory browser, light/dark)
- `hwt` — live TUI sensors panel (current/min/max/avg per sensor, grouped
  by chip)
- `hwtctl` — scripting CLI (query, JSON output, stats reset, history,
  log control, alert state, report export)


Sensor identities are derived from stable device topology
(`hwmon:pci-0000:00:18.3:temp1`), never from kernel enumeration order, so
they survive reboots and kernel updates.

## Building

Requires Go 1.24+.

```
go build ./cmd/hwtd ./cmd/hwt ./cmd/hwtctl
```

The GUI needs CGO and OpenGL/X11/Wayland headers (all other binaries are
pure-Go, CGO_ENABLED=0):

```
# Fedora
sudo dnf install gcc mesa-libGL-devel libX11-devel libXcursor-devel \
  libXrandr-devel libXinerama-devel libXi-devel libXxf86vm-devel \
  wayland-devel libxkbcommon-devel wayland-protocols-devel
# Debian/Ubuntu
sudo apt install gcc libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev

go build ./cmd/hwt-gui
```

## Running

Start the daemon (root recommended — SMBIOS inventory and some sensors need
it; without root it degrades gracefully):

```
./hwtd
```

Then, in another terminal:

```
./hwt                        # live TUI: q quit, r reset stats, ↑/↓ scroll
./hwtctl sensors             # one-shot table
./hwtctl sensors -json       # machine-readable
./hwtctl devices             # full inventory: DMI, PCI, USB, monitors, drives
./hwtctl history -id <id>    # buffered samples for one sensor
./hwtctl alerts              # state of configured alert rules
curl localhost:11988/metrics # Prometheus/OpenMetrics
```

### Reports

Export the full inventory + sensor state (HWiNFO report parity), in five
formats. `-redact` blanks serials/UUIDs/MACs so reports are safe to share:

```
./hwtctl report -format html -redact -o report.html
./hwtctl report -format text -section pci,usb
./hwtctl report -format json | jq .
```

Point Prometheus at `localhost:11988` and graph `hwt_temp_celsius`,
`hwt_fan_rpm`, `hwt_freq_hertz`, ... in Grafana. `hwt_provider_up` reports
provider health, `hwt_alert_firing` mirrors alert state.

### Logging

HWiNFO-style sensor logging (one row per poll tick), controlled at runtime:

```
./hwtctl log start -path run.csv       # or -format ndjson
./hwtctl log mark "started benchmark"  # annotate the next row
./hwtctl log stop
```

### Alerts

Rules live in `/etc/hw-t/config.toml` and fire after a sustained breach,
with hysteresis on the way down:

```toml
[[alert]]
name = "cpu-hot"
sensor = "hwmon:pci-0000:00:18.3:temp1"
above = 90.0
for = "10s"
hysteresis = 5.0
actions = ["journal", "notify", "exec:/usr/local/bin/panic.sh", "webhook:http://127.0.0.1:9000/hook"]
```

Poll cadences (`[poll] fast/medium/slow`), history span, and the default
log path are also set there; every setting has a sane default.

A hardened systemd unit lives in `packaging/systemd/hwtd.service`.

## Development

Providers read from a configurable sysfs root, so captured fixture trees run
the same code paths as real hardware:

```
go test ./...
./hwtd -sysfs testdata/fixtures/basic/sys
```

## License

MIT — see [LICENSE](LICENSE).
