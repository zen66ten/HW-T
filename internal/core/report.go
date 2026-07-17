package core

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ReportOptions selects report format and content (§6.3).
type ReportOptions struct {
	Format   string   // text | html | json | yaml | csv
	Sections []string // provider names; empty = all
	Redact   bool     // blank serials, UUIDs, MACs for public sharing
}

// redactedKeys are attr-name substrings whose values are hidden by
// --redact.
var redactedKeys = []string{"serial", "uuid", "mac"}

// ReportDevice is one device with its live sensor state attached.
type ReportDevice struct {
	ID       string            `json:"id" yaml:"id"`
	Provider string            `json:"provider" yaml:"provider"`
	Name     string            `json:"name" yaml:"name"`
	Attrs    map[string]string `json:"attrs,omitempty" yaml:"attrs,omitempty"`
	Sensors  []ReportSensor    `json:"sensors,omitempty" yaml:"sensors,omitempty"`
}

// ReportSensor is one channel's formatted state.
type ReportSensor struct {
	Label string  `json:"label" yaml:"label"`
	Value string  `json:"value" yaml:"value"`
	Cur   float64 `json:"cur" yaml:"cur"`
	Min   float64 `json:"min" yaml:"min"`
	Max   float64 `json:"max" yaml:"max"`
	Avg   float64 `json:"avg" yaml:"avg"`
	Err   string  `json:"err,omitempty" yaml:"err,omitempty"`
}

// Report is the exportable document.
type Report struct {
	Generated time.Time      `json:"generated" yaml:"generated"`
	Host      string         `json:"host" yaml:"host"`
	Devices   []ReportDevice `json:"devices" yaml:"devices"`
}

// BuildReport assembles and encodes a report from the registry state.
func BuildReport(devs []Device, sensors []Sensor, opts ReportOptions) ([]byte, error) {
	r := assemble(devs, sensors, opts)
	switch opts.Format {
	case "", "text":
		return renderText(r), nil
	case "json":
		return json.MarshalIndent(r, "", "  ")
	case "yaml":
		return yaml.Marshal(r)
	case "csv":
		return renderCSV(r)
	case "html":
		return renderHTML(r)
	default:
		return nil, fmt.Errorf("unknown report format %q", opts.Format)
	}
}

func assemble(devs []Device, sensors []Sensor, opts ReportOptions) *Report {
	include := map[string]bool{}
	for _, s := range opts.Sections {
		include[s] = true
	}

	byDevice := map[DeviceID][]ReportSensor{}
	for _, s := range sensors {
		channel := strings.TrimPrefix(string(s.ID), string(s.Device)+":")
		rs := ReportSensor{
			Label: EnrichLabel(s.DeviceName, channel, s.Label),
			Cur:   s.Cur, Min: s.Min, Max: s.Max, Avg: s.Avg,
			Err: s.Err,
		}
		if s.Err == "" && s.N > 0 {
			rs.Value = FormatValue(s.Kind, s.Cur)
		}
		byDevice[s.Device] = append(byDevice[s.Device], rs)
	}

	host, _ := os.Hostname()
	r := &Report{Generated: time.Now(), Host: host}
	for _, d := range devs {
		if len(include) > 0 && !include[d.Provider] {
			continue
		}
		rd := ReportDevice{
			ID:       string(d.ID),
			Provider: d.Provider,
			Name:     d.Name,
			Attrs:    map[string]string{},
			Sensors:  byDevice[d.ID],
		}
		for k, v := range d.Attrs {
			if opts.Redact && isRedacted(k) {
				v = "[redacted]"
			}
			rd.Attrs[k] = v
		}
		r.Devices = append(r.Devices, rd)
	}
	return r
}

func isRedacted(key string) bool {
	k := strings.ToLower(key)
	for _, sub := range redactedKeys {
		if strings.Contains(k, sub) {
			return true
		}
	}
	return false
}

func sortedAttrKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func renderText(r *Report) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "HW-T report, %s, host %s\n", r.Generated.Format(time.RFC3339), r.Host)

	lastProvider := ""
	for _, d := range r.Devices {
		if d.Provider != lastProvider {
			lastProvider = d.Provider
			fmt.Fprintf(&b, "\n===== %s =====\n", d.Provider)
		}
		fmt.Fprintf(&b, "\n%s  [%s]\n", DisplayName(d.Name), d.ID)
		for _, k := range sortedAttrKeys(d.Attrs) {
			fmt.Fprintf(&b, "  %-18s %s\n", k+":", d.Attrs[k])
		}
		for _, s := range d.Sensors {
			if s.Err != "" {
				fmt.Fprintf(&b, "  %-18s N/A (%s)\n", s.Label+":", s.Err)
				continue
			}
			fmt.Fprintf(&b, "  %-18s %s  (min %g, max %g, avg %.1f)\n", s.Label+":", s.Value, s.Min, s.Max, s.Avg)
		}
	}
	return []byte(b.String())
}

func renderCSV(r *Report) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Write([]string{"kind", "device", "provider", "name", "key", "value", "min", "max", "avg"})
	for _, d := range r.Devices {
		for _, k := range sortedAttrKeys(d.Attrs) {
			w.Write([]string{"attr", d.ID, d.Provider, d.Name, k, d.Attrs[k], "", "", ""})
		}
		for _, s := range d.Sensors {
			w.Write([]string{"sensor", d.ID, d.Provider, d.Name, s.Label, s.Value,
				strconv.FormatFloat(s.Min, 'f', -1, 64),
				strconv.FormatFloat(s.Max, 'f', -1, 64),
				strconv.FormatFloat(s.Avg, 'f', -1, 64)})
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

// htmlTemplate is the single-file report (§6.3): no external assets, safe
// to attach to a support-forum post.
var htmlTemplate = template.Must(template.New("report").Funcs(template.FuncMap{
	"display":  DisplayName,
	"attrKeys": sortedAttrKeys,
}).Parse(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8">
<title>HW-T report: {{.Host}}</title>
<style>
body { font: 14px/1.5 system-ui, sans-serif; background: #14161a; color: #d8dce2; margin: 2rem auto; max-width: 60rem; padding: 0 1rem; }
h1 { font-size: 1.4rem; } h2 { font-size: 1.1rem; border-bottom: 1px solid #2a2e35; padding-bottom: .3rem; margin-top: 2rem; color: #7fb4e6; }
h3 { font-size: 1rem; margin: 1.2rem 0 .3rem; }
table { border-collapse: collapse; width: 100%; margin-bottom: .8rem; }
td { padding: .18rem .6rem; border: 1px solid #2a2e35; }
td.k { color: #8b93a0; width: 14rem; }
small { color: #8b93a0; }
</style></head><body>
<h1>HW-T hardware report</h1>
<small>generated {{.Generated.Format "2006-01-02 15:04:05"}} on {{.Host}}</small>
{{$last := ""}}
{{range .Devices}}
{{if ne .Provider $last}}{{$last = .Provider}}<h2>{{.Provider}}</h2>{{end}}
<h3>{{display .Name}} <small>[{{.ID}}]</small></h3>
<table>
{{$d := .}}{{range attrKeys .Attrs}}<tr><td class="k">{{.}}</td><td>{{index $d.Attrs .}}</td></tr>
{{end}}{{range .Sensors}}<tr><td class="k">{{.Label}}</td><td>{{if .Err}}N/A <small>({{.Err}})</small>{{else}}{{.Value}} <small>min {{printf "%g" .Min}} · max {{printf "%g" .Max}} · avg {{printf "%.1f" .Avg}}</small>{{end}}</td></tr>
{{end}}
</table>
{{end}}
</body></html>
`))

func renderHTML(r *Report) ([]byte, error) {
	var buf bytes.Buffer
	if err := htmlTemplate.Execute(&buf, r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
