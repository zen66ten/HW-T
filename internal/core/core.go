// Package core is the daemon heart: the sensor registry, the poll
// scheduler, per-sensor ring buffers and running statistics. Providers feed
// it; every API surface reads from it.
package core

import (
	"context"
	"fmt"
	"time"
)

// Kind classifies a sensor channel and determines its unit.
type Kind string

const (
	KindIn       Kind = "in"
	KindFan      Kind = "fan"
	KindPWM      Kind = "pwm"
	KindTemp     Kind = "temp"
	KindCurr     Kind = "curr"
	KindPower    Kind = "power"
	KindEnergy   Kind = "energy"
	KindHumidity Kind = "humidity"
	KindFreq     Kind = "freq"
)

// Unit returns the presentation unit for a kind.
func (k Kind) Unit() string {
	switch k {
	case KindTemp:
		return "°C"
	case KindIn:
		return "V"
	case KindFan:
		return "RPM"
	case KindCurr:
		return "A"
	case KindPower:
		return "W"
	case KindEnergy:
		return "J"
	case KindHumidity:
		return "%RH"
	case KindFreq:
		return "MHz"
	default:
		return ""
	}
}

// FormatValue renders a value the way lm-sensors would.
func FormatValue(k Kind, v float64) string {
	switch k {
	case KindTemp:
		return fmt.Sprintf("%+.1f°C", v)
	case KindIn:
		return fmt.Sprintf("%+.3f V", v)
	case KindFan:
		return fmt.Sprintf("%.0f RPM", v)
	case KindCurr:
		return fmt.Sprintf("%+.3f A", v)
	case KindPower:
		return fmt.Sprintf("%.2f W", v)
	case KindEnergy:
		return fmt.Sprintf("%.2f J", v)
	case KindHumidity:
		return fmt.Sprintf("%.1f %%RH", v)
	case KindFreq:
		return fmt.Sprintf("%.0f MHz", v)
	default:
		return fmt.Sprintf("%.0f", v)
	}
}

// DeviceID identifies a device by provider and stable topology path,
// e.g. "hwmon:pci-0000:00:18.3". Never derived from unstable kernel indexes.
type DeviceID string

// SensorID is DeviceID plus channel, e.g. "hwmon:pci-0000:00:18.3:temp1".
// User customizations and history key off this and survive reboots.
type SensorID string

// ChannelInfo describes one sensor channel on a device.
type ChannelInfo struct {
	ID     string             `json:"id"`
	Kind   Kind               `json:"kind"`
	Label  string             `json:"label"`
	Limits map[string]float64 `json:"limits,omitempty"`
}

// Device is one discovered hardware device. Inventory-only devices (DMI)
// carry Attrs and no channels.
type Device struct {
	ID       DeviceID          `json:"id"`
	Provider string            `json:"provider"`
	Name     string            `json:"name"`
	Attrs    map[string]string `json:"attrs,omitempty"`
	Channels []ChannelInfo     `json:"channels,omitempty"`
}

// Reading is one collected channel value.
type Reading struct {
	Channel string
	Value   float64
	Err     string
}

// Provider is the contract every data source implements. Discovery is slow
// and allocating; Collect is the hot path. A panicking provider is
// quarantined by the scheduler, never crashes the daemon.
type Provider interface {
	Name() string
	Discover(ctx context.Context) ([]Device, error)
	Collect(ctx context.Context, dev DeviceID) ([]Reading, error)
	DefaultInterval() time.Duration // 0 means static: discover once, never collect
}

// Sensor is the merged info+state view served to clients.
type Sensor struct {
	ID         SensorID           `json:"id"`
	Device     DeviceID           `json:"device"`
	DeviceName string             `json:"device_name"`
	Provider   string             `json:"provider"`
	Kind       Kind               `json:"kind"`
	Label      string             `json:"label"`
	Unit       string             `json:"unit"`
	Limits     map[string]float64 `json:"limits,omitempty"`

	Cur float64   `json:"cur"`
	Min float64   `json:"min"`
	Max float64   `json:"max"`
	Avg float64   `json:"avg"`
	N   uint64    `json:"n"`
	Ts  time.Time `json:"ts"`
	Err string    `json:"err,omitempty"`
}

// Point is one history sample.
type Point struct {
	Ts    int64   `json:"ts"` // unix milliseconds
	Value float64 `json:"v"`
}
