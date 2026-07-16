// Package metrics exposes the registry as Prometheus/OpenMetrics gauges.
// Metric names are stable API (SPEC §7): hwt_temp_celsius, hwt_fan_rpm, ...
// labeled by stable sensor id, chip name and channel label.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/zen66ten/HW-T/internal/core"
)

var kindMetrics = map[core.Kind]string{
	core.KindTemp:     "hwt_temp_celsius",
	core.KindIn:       "hwt_in_volts",
	core.KindFan:      "hwt_fan_rpm",
	core.KindPWM:      "hwt_pwm",
	core.KindCurr:     "hwt_curr_amps",
	core.KindPower:    "hwt_power_watts",
	core.KindEnergy:   "hwt_energy_joules",
	core.KindHumidity: "hwt_humidity_percent",
	core.KindFreq:     "hwt_freq_hertz",
}

var sensorLabels = []string{"id", "chip", "label"}

type collector struct {
	reg   *core.Registry
	descs map[core.Kind]*prometheus.Desc
	up    *prometheus.Desc
}

// Handler returns the /metrics HTTP handler backed by the registry.
func Handler(reg *core.Registry) http.Handler {
	c := &collector{
		reg:   reg,
		descs: map[core.Kind]*prometheus.Desc{},
		up: prometheus.NewDesc("hwt_provider_up",
			"1 when the provider is healthy, 0 when quarantined or failed.",
			[]string{"provider"}, nil),
	}
	for kind, name := range kindMetrics {
		c.descs[kind] = prometheus.NewDesc(name,
			"HW-T sensor value ("+string(kind)+").", sensorLabels, nil)
	}
	promReg := prometheus.NewRegistry()
	promReg.MustRegister(c)
	return promhttp.HandlerFor(promReg, promhttp.HandlerOpts{})
}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range c.descs {
		ch <- d
	}
	ch <- c.up
}

func (c *collector) Collect(ch chan<- prometheus.Metric) {
	quarantined := c.reg.Quarantined()
	providers := map[string]bool{}

	for _, s := range c.reg.Snapshot() {
		providers[s.Provider] = true
		if s.Err != "" || s.N == 0 {
			continue
		}
		desc, ok := c.descs[s.Kind]
		if !ok {
			continue
		}
		v := s.Cur
		if s.Kind == core.KindFreq {
			v *= 1e6 // MHz -> Hz, Prometheus base units
		}
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v,
			string(s.ID), s.DeviceName, s.Label)
	}

	for _, d := range c.reg.Devices() {
		providers[d.Provider] = true
	}
	for p := range providers {
		up := 1.0
		if _, bad := quarantined[p]; bad {
			up = 0
		}
		ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, up, p)
	}
	for p := range quarantined {
		if !providers[p] {
			ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 0, p)
		}
	}
}
