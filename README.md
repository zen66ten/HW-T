# HW-T

A HWiNFO64-class hardware inventory and sensor monitoring suite for Linux.

## Overview

HW-T is a hardware monitoring application for Linux that combines deep hardware inventory with real-time sensor monitoring. It aggregates data from kernel interfaces (`hwmon`, DMI/SMBIOS, NVML, DRM sysfs, NVMe, SMART) and presents it through a TUI and scripting CLI.

**Components:**
- `hwtd` — background daemon
- `hwt` — TUI client
- `hwtctl` — scripting CLI

## Status

Phase 0 spike: `hwt` enumerates hwmon chips and cpufreq from sysfs and prints a
sensors-style table with labels, values, and thresholds.

## Building

Requires Go 1.24+.

```
go build ./cmd/hwt
./hwt
```

Providers read from a configurable sysfs root, so captured fixture trees run
the same code paths as real hardware:

```
go test ./...
./hwt -sysfs testdata/fixtures/basic/sys
```

## License

_License to be decided._
