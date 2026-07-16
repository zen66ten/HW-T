package core

import (
	"sync"
	"time"
)

// Stats holds running min/max/avg since the last reset, HWiNFO-style.
type Stats struct {
	Min, Max, Sum float64
	N             uint64
}

func (s *Stats) Add(v float64) {
	if s.N == 0 || v < s.Min {
		s.Min = v
	}
	if s.N == 0 || v > s.Max {
		s.Max = v
	}
	s.Sum += v
	s.N++
}

func (s *Stats) Avg() float64 {
	if s.N == 0 {
		return 0
	}
	return s.Sum / float64(s.N)
}

type sensorState struct {
	device  DeviceID
	channel ChannelInfo
	cur     float64
	ts      time.Time
	err     string
	stats   Stats
	ring    *Ring
}

// Registry is the central sensor store. Devices are registered per provider
// after discovery; readings stream in via Apply. Because sensor IDs are
// topology-stable, re-discovery (hotplug, daemon restart) transparently
// re-attaches existing state.
type Registry struct {
	mu          sync.RWMutex
	ringCap     int
	devices     []Device // discovery order, grouped by provider
	sensors     map[SensorID]*sensorState
	order       []SensorID
	quarantined map[string]string // provider -> reason
}

func NewRegistry(ringCap int) *Registry {
	return &Registry{
		ringCap:     ringCap,
		sensors:     map[SensorID]*sensorState{},
		quarantined: map[string]string{},
	}
}

func sensorID(dev DeviceID, channel string) SensorID {
	return SensorID(string(dev) + ":" + channel)
}

// SetDevices replaces one provider's device set, preserving stats and
// history for sensor IDs that persist across the re-discovery.
func (r *Registry) SetDevices(provider string, devs []Device) {
	r.mu.Lock()
	defer r.mu.Unlock()

	kept := r.devices[:0]
	for _, d := range r.devices {
		if d.Provider != provider {
			kept = append(kept, d)
		}
	}
	r.devices = append(kept, devs...)

	live := map[SensorID]bool{}
	for _, d := range devs {
		for _, ch := range d.Channels {
			id := sensorID(d.ID, ch.ID)
			live[id] = true
			if st, ok := r.sensors[id]; ok {
				st.channel = ch
				continue
			}
			r.sensors[id] = &sensorState{
				device:  d.ID,
				channel: ch,
				ring:    NewRing(r.ringCap),
			}
		}
	}
	for id, st := range r.sensors {
		if !live[id] {
			for _, d := range r.devices {
				if d.ID == st.device && d.Provider == provider {
					delete(r.sensors, id)
				}
			}
		}
	}
	r.rebuildOrder()
}

func (r *Registry) rebuildOrder() {
	r.order = r.order[:0]
	for _, d := range r.devices {
		for _, ch := range d.Channels {
			r.order = append(r.order, sensorID(d.ID, ch.ID))
		}
	}
}

// Apply records one collection pass for a device.
func (r *Registry) Apply(dev DeviceID, readings []Reading, ts time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rd := range readings {
		st, ok := r.sensors[sensorID(dev, rd.Channel)]
		if !ok {
			continue
		}
		st.ts = ts
		st.err = rd.Err
		if rd.Err != "" {
			continue
		}
		st.cur = rd.Value
		st.stats.Add(rd.Value)
		st.ring.Push(ts, rd.Value)
	}
}

// Snapshot returns all sensors in stable display order.
func (r *Registry) Snapshot() []Sensor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	devByID := map[DeviceID]*Device{}
	for i := range r.devices {
		devByID[r.devices[i].ID] = &r.devices[i]
	}

	out := make([]Sensor, 0, len(r.order))
	for _, id := range r.order {
		st := r.sensors[id]
		dev := devByID[st.device]
		out = append(out, Sensor{
			ID:         id,
			Device:     st.device,
			DeviceName: dev.Name,
			Provider:   dev.Provider,
			Kind:       st.channel.Kind,
			Label:      st.channel.Label,
			Unit:       st.channel.Kind.Unit(),
			Limits:     st.channel.Limits,
			Cur:        st.cur,
			Min:        st.stats.Min,
			Max:        st.stats.Max,
			Avg:        st.stats.Avg(),
			N:          st.stats.N,
			Ts:         st.ts,
			Err:        st.err,
		})
	}
	return out
}

// Devices returns all discovered devices, including channel-less inventory
// devices (DMI).
func (r *Registry) Devices() []Device {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Device(nil), r.devices...)
}

// History returns the ring buffer contents for one sensor.
func (r *Registry) History(id SensorID) ([]Point, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	st, ok := r.sensors[id]
	if !ok {
		return nil, false
	}
	return st.ring.Points(), true
}

// Reset clears running stats for one sensor, or all when id is empty.
func (r *Registry) Reset(id SensorID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id == "" {
		for _, st := range r.sensors {
			st.stats = Stats{}
		}
		return true
	}
	st, ok := r.sensors[id]
	if !ok {
		return false
	}
	st.stats = Stats{}
	return true
}

// SetQuarantined records a provider failure (see scheduler).
func (r *Registry) SetQuarantined(provider, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.quarantined[provider] = reason
}

// Quarantined returns provider -> reason for all quarantined providers.
func (r *Registry) Quarantined() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.quarantined))
	for k, v := range r.quarantined {
		out[k] = v
	}
	return out
}
