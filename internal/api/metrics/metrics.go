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
	core.KindHealth:   "hwt_health_ok",
	core.KindPercent:  "hwt_percent",
	core.KindCount:    "hwt_count",
	core.KindData:     "hwt_data_bytes",
}

var sensorLabels = []string{"id", "chip", "label"}

type collector struct {
	reg    *core.Registry
	descs  map[core.Kind]*prometheus.Desc
	up     *prometheus.Desc
	alert  *prometheus.Desc
	alerts func() []core.AlertStatus // nil when no alert engine configured
}

// Handler returns the /metrics HTTP handler backed by the registry.
// alerts may be nil; when set, each rule exports hwt_alert_firing{rule}.
func Handler(reg *core.Registry, alerts func() []core.AlertStatus) http.Handler {
	c := &collector{
		reg:    reg,
		descs:  map[core.Kind]*prometheus.Desc{},
		alerts: alerts,
		up: prometheus.NewDesc("hwt_provider_up",
			"1 when the provider is healthy, 0 when quarantined or failed.",
			[]string{"provider"}, nil),
		alert: prometheus.NewDesc("hwt_alert_firing",
			"1 while the alert rule is firing, 0 otherwise.",
			[]string{"rule", "sensor"}, nil),
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
	ch <- c.alert
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
		switch s.Kind {
		case core.KindFreq:
			v *= 1e6 // MHz -> Hz, Prometheus base units
		case core.KindData:
			v *= 1 << 20 // MiB -> bytes
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

	if c.alerts != nil {
		for _, a := range c.alerts() {
			v := 0.0
			if a.State == core.AlertFiring {
				v = 1
			}
			ch <- prometheus.MustNewConstMetric(c.alert, prometheus.GaugeValue, v, a.Name, string(a.Sensor))
		}
	}
}
