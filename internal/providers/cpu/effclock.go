package cpu

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// effClock reads per-core effective (actual) clock speed from the APERF and
// MPERF MSRs, per §5.2's preferred method: perf_event_open on the "msr" PMU
// (no /dev/cpu/*/msr or CAP_SYS_RAWIO needed, just CAP_PERFMON/root).
//
// APERF increments at the true current core frequency while the core is in
// C0; MPERF increments at the fixed nominal (P0/invariant-TSC) rate, also
// only in C0. So effective_freq = (dAPERF/dMPERF) * TSC_freq, where TSC_freq
// is measured once via a short calibration (invariant TSC ticks at a
// constant rate always, so a busy-loop-free wall-clock/TSC-delta sample is
// enough — no CPUID leaf 0x15 parsing needed).
type effClock struct {
	tscHz float64
	cores map[int]*coreCounters
}

type coreCounters struct {
	aperfFd, mperfFd     int
	lastAperf, lastMperf uint64
}

const msrPMUPath = "/sys/bus/event_source/devices/msr"

// newEffClock sets up MSR-PMU counters for every given CPU. It returns
// (nil, err) if the msr PMU is absent (non-x86, ancient kernel) or the
// process lacks permission (CAP_PERFMON/root) — callers treat that as
// "feature unavailable", not fatal, since scaling_cur_freq still works.
func newEffClock(cpus []int) (*effClock, error) {
	pmuType, err := readMSRPMUType()
	if err != nil {
		return nil, fmt.Errorf("msr PMU not available: %w", err)
	}
	aperfCfg, err := readMSREventConfig("aperf")
	if err != nil {
		return nil, err
	}
	mperfCfg, err := readMSREventConfig("mperf")
	if err != nil {
		return nil, err
	}
	tscCfg, err := readMSREventConfig("tsc")
	if err != nil {
		return nil, err
	}

	tscHz, err := calibrateTSC(pmuType, tscCfg, cpus[0])
	if err != nil {
		return nil, fmt.Errorf("tsc calibration: %w", err)
	}

	ec := &effClock{tscHz: tscHz, cores: map[int]*coreCounters{}}
	for _, cpuN := range cpus {
		aFd, err := openPerfCounter(pmuType, aperfCfg, cpuN)
		if err != nil {
			ec.Close()
			return nil, fmt.Errorf("open aperf cpu%d: %w", cpuN, err)
		}
		mFd, err := openPerfCounter(pmuType, mperfCfg, cpuN)
		if err != nil {
			unix.Close(aFd)
			ec.Close()
			return nil, fmt.Errorf("open mperf cpu%d: %w", cpuN, err)
		}
		cc := &coreCounters{aperfFd: aFd, mperfFd: mFd}
		cc.lastAperf, _ = readPerfCounter(aFd)
		cc.lastMperf, _ = readPerfCounter(mFd)
		ec.cores[cpuN] = cc
	}
	return ec, nil
}

func (ec *effClock) Close() {
	for _, cc := range ec.cores {
		unix.Close(cc.aperfFd)
		unix.Close(cc.mperfFd)
	}
}

// Read returns the effective MHz for one CPU since the previous Read/setup.
func (ec *effClock) Read(cpuN int) (float64, error) {
	cc, ok := ec.cores[cpuN]
	if !ok {
		return 0, fmt.Errorf("cpu%d: no effective-clock counters", cpuN)
	}
	aperf, err := readPerfCounter(cc.aperfFd)
	if err != nil {
		return 0, err
	}
	mperf, err := readPerfCounter(cc.mperfFd)
	if err != nil {
		return 0, err
	}
	dAperf := aperf - cc.lastAperf
	dMperf := mperf - cc.lastMperf
	cc.lastAperf, cc.lastMperf = aperf, mperf
	if dMperf == 0 {
		return 0, nil // core was fully idle (C-state) for the whole interval
	}
	return (float64(dAperf) / float64(dMperf)) * ec.tscHz / 1e6, nil
}

func calibrateTSC(pmuType uint32, tscCfg uint64, cpuN int) (float64, error) {
	fd, err := openPerfCounter(pmuType, tscCfg, cpuN)
	if err != nil {
		return 0, err
	}
	defer unix.Close(fd)

	t0 := time.Now()
	v0, err := readPerfCounter(fd)
	if err != nil {
		return 0, err
	}
	time.Sleep(50 * time.Millisecond)
	v1, err := readPerfCounter(fd)
	if err != nil {
		return 0, err
	}
	dt := time.Since(t0).Seconds()
	if dt <= 0 || v1 <= v0 {
		return 0, fmt.Errorf("degenerate TSC calibration sample")
	}
	return float64(v1-v0) / dt, nil
}

func openPerfCounter(pmuType uint32, config uint64, cpuN int) (int, error) {
	attr := unix.PerfEventAttr{
		Type:   pmuType,
		Size:   uint32(unsafe.Sizeof(unix.PerfEventAttr{})),
		Config: config,
	}
	fd, err := unix.PerfEventOpen(&attr, -1, cpuN, -1, 0)
	if err != nil {
		return 0, err
	}
	return fd, nil
}

func readPerfCounter(fd int) (uint64, error) {
	var buf [8]byte
	n, err := unix.Read(fd, buf[:])
	if err != nil {
		return 0, err
	}
	if n != 8 {
		return 0, fmt.Errorf("short read (%d bytes)", n)
	}
	return uint64(buf[0]) | uint64(buf[1])<<8 | uint64(buf[2])<<16 | uint64(buf[3])<<24 |
		uint64(buf[4])<<32 | uint64(buf[5])<<40 | uint64(buf[6])<<48 | uint64(buf[7])<<56, nil
}

func readMSRPMUType() (uint32, error) {
	s, err := readString(msrPMUPath + "/type")
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseUint(s, 10, 32)
	return uint32(n), err
}

// readMSREventConfig parses the "event=0xNN" format sysfs exposes for each
// msr PMU event (aperf, mperf, tsc, irperf).
func readMSREventConfig(name string) (uint64, error) {
	s, err := readString(msrPMUPath + "/events/" + name)
	if err != nil {
		return 0, err
	}
	_, hex, found := strings.Cut(s, "event=")
	if !found {
		return 0, fmt.Errorf("unexpected msr event format for %s: %q", name, s)
	}
	return strconv.ParseUint(strings.TrimPrefix(hex, "0x"), 16, 64)
}
