# HW-T

A HWiNFO64-class hardware inventory and sensor monitoring suite for Linux.

## Overview

HW-T is a hardware monitoring suite for Linux that combines deep hardware
inventory with real-time sensor monitoring. It aggregates data from kernel
interfaces (`hwmon`, cpufreq, DMI/SMBIOS, and more to come) and serves it
through a TUI, a scripting CLI, a Unix-socket API, and Prometheus metrics.

**Components:**
- `hwtd` — monitoring daemon: discovers hardware, polls sensors, tracks
  min/max/avg and history, serves all APIs
- `hwt` — live TUI sensors panel (current/min/max/avg per sensor, grouped
  by chip)
- `hwtctl` — scripting CLI (query, JSON output, stats reset, history)

Sensor identities are derived from stable device topology
(`hwmon:pci-0000:00:18.3:temp1`), never from kernel enumeration order, so
they survive reboots and kernel updates.

## Building

Requires Go 1.24+.

```
go build ./cmd/hwtd ./cmd/hwt ./cmd/hwtctl
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
./hwtctl devices             # inventory incl. DMI (BIOS, board, RAM)
./hwtctl history -id <id>    # buffered samples for one sensor
curl localhost:11988/metrics # Prometheus/OpenMetrics
```

Point Prometheus at `localhost:11988` and graph `hwt_temp_celsius`,
`hwt_fan_rpm`, `hwt_freq_hertz`, ... in Grafana. `hwt_provider_up` reports
provider health.

A hardened systemd unit lives in `packaging/systemd/hwtd.service`.

## Development

Providers read from a configurable sysfs root, so captured fixture trees run
the same code paths as real hardware:

```
go test ./...
./hwtd -sysfs testdata/fixtures/basic/sys
```

## License

_License to be decided._
