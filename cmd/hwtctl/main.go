// hwtctl is the scripting CLI: query the daemon, dump JSON, reset stats.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/zen66ten/HW-T/internal/core"
	"github.com/zen66ten/HW-T/pkg/client"
)

const usage = `usage: hwtctl [-socket path] <command>

commands:
  sensors [-json]         current values with min/max/avg
  devices [-json]         discovered devices incl. DMI inventory
  history -id <id>        buffered samples for one sensor (JSON)
  reset [-id <id>]        reset min/max/avg (all sensors when -id omitted)
  alerts [-json]          state of configured alert rules
  log start [-path p] [-format csv|ndjson] [-sensors id,id,...]
  log stop
  log mark <note>         attach a note to the next logged row
  log status
  report [-format text|html|json|yaml|csv] [-section pci,usb,...] [-redact] [-o file]
`

func main() {
	socket := flag.String("socket", client.DefaultSocket(), "hwtd unix socket")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	c, err := client.Dial(*socket)
	if err != nil {
		fatal(err)
	}
	defer c.Close()

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "sensors":
		runSensors(c, rest)
	case "devices":
		runDevices(c, rest)
	case "history":
		runHistory(c, rest)
	case "reset":
		runReset(c, rest)
	case "alerts":
		runAlerts(c, rest)
	case "log":
		runLog(c, rest)
	case "report":
		runReport(c, rest)
	default:
		flag.Usage()
		os.Exit(2)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "hwtctl:", err)
	os.Exit(1)
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func runSensors(c *client.Client, args []string) {
	fs := flag.NewFlagSet("sensors", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "output JSON")
	fs.Parse(args)

	sensors, err := c.Sensors()
	if err != nil {
		fatal(err)
	}
	if *asJSON {
		printJSON(sensors)
		return
	}

	lastDev := ""
	fmt.Printf("%-26s%12s%12s%12s%12s\n", "Sensor", "Current", "Min", "Max", "Avg")
	for _, s := range sensors {
		if s.Device != lastDev {
			lastDev = s.Device
			path := core.ShortenPath(strings.TrimPrefix(s.Device, s.Provider+":"), 50)
			fmt.Printf("%s  (%s)\n", core.DisplayName(s.DeviceName), path)
		}
		label := core.EnrichLabel(s.DeviceName, strings.TrimPrefix(s.ID, s.Device+":"), s.Label)
		if s.Err != "" {
			fmt.Printf("  %-24s%12s\n", label, "N/A")
			continue
		}
		kind := core.Kind(s.Kind)
		fmt.Printf("  %-24s%12s%12s%12s%12s\n", label,
			core.FormatValue(kind, s.Cur),
			core.FormatValue(kind, s.Min),
			core.FormatValue(kind, s.Max),
			core.FormatValue(kind, s.Avg))
	}
}

func runDevices(c *client.Client, args []string) {
	fs := flag.NewFlagSet("devices", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "output JSON")
	fs.Parse(args)

	devices, err := c.Devices()
	if err != nil {
		fatal(err)
	}
	if *asJSON {
		printJSON(devices)
		return
	}
	for _, d := range devices {
		fmt.Printf("%s  [%s]\n", core.DisplayName(d.Name), d.ID)
		for _, k := range sortedKeys(d.Attrs) {
			fmt.Printf("  %-16s %s\n", k, d.Attrs[k])
		}
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func runHistory(c *client.Client, args []string) {
	fs := flag.NewFlagSet("history", flag.ExitOnError)
	id := fs.String("id", "", "sensor id")
	fs.Parse(args)
	if *id == "" {
		fatal(fmt.Errorf("history requires -id"))
	}
	points, err := c.History(*id)
	if err != nil {
		fatal(err)
	}
	printJSON(points)
}

func runReport(c *client.Client, args []string) {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	format := fs.String("format", "text", "text, html, json, yaml, or csv")
	sections := fs.String("section", "", "comma-separated provider filter (e.g. pci,usb,dmi)")
	redact := fs.Bool("redact", false, "hide serials, UUIDs and MACs")
	out := fs.String("o", "", "write to file instead of stdout")
	fs.Parse(args)

	var secs []string
	if *sections != "" {
		secs = strings.Split(*sections, ",")
	}
	report, err := c.Report(*format, secs, *redact)
	if err != nil {
		fatal(err)
	}
	if *out == "" {
		fmt.Print(report)
		return
	}
	if err := os.WriteFile(*out, []byte(report), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("report written to %s (%d bytes)\n", *out, len(report))
}

func runReset(c *client.Client, args []string) {
	fs := flag.NewFlagSet("reset", flag.ExitOnError)
	id := fs.String("id", "", "sensor id (all when empty)")
	fs.Parse(args)
	if err := c.Reset(*id); err != nil {
		fatal(err)
	}
	fmt.Println("ok")
}

func runAlerts(c *client.Client, args []string) {
	fs := flag.NewFlagSet("alerts", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "output JSON")
	fs.Parse(args)

	alerts, err := c.Alerts()
	if err != nil {
		fatal(err)
	}
	if *asJSON {
		printJSON(alerts)
		return
	}
	if len(alerts) == 0 {
		fmt.Println("no alert rules configured")
		return
	}
	fmt.Printf("%-20s%-10s%12s  %s\n", "Rule", "State", "Value", "Sensor")
	for _, a := range alerts {
		fmt.Printf("%-20s%-10s%12.1f  %s\n", a.Name, a.State, a.Value, a.Sensor)
	}
}

func runLog(c *client.Client, args []string) {
	if len(args) == 0 {
		fatal(fmt.Errorf("log requires a subcommand: start | stop | mark | status"))
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "start":
		fs := flag.NewFlagSet("log start", flag.ExitOnError)
		path := fs.String("path", "", "log file (daemon default when empty)")
		format := fs.String("format", "csv", "csv or ndjson")
		sensorList := fs.String("sensors", "", "comma-separated sensor IDs (all when empty)")
		fs.Parse(rest)
		var sensors []string
		if *sensorList != "" {
			sensors = strings.Split(*sensorList, ",")
		}
		st, err := c.LogStart(*path, *format, sensors)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("logging to %s (%s)\n", st.Path, st.Format)
	case "stop":
		if err := c.LogStop(); err != nil {
			fatal(err)
		}
		fmt.Println("logging stopped")
	case "mark":
		if len(rest) == 0 {
			fatal(fmt.Errorf(`log mark requires a note argument`))
		}
		if err := c.LogMark(strings.Join(rest, " ")); err != nil {
			fatal(err)
		}
		fmt.Println("ok")
	case "status":
		st, err := c.LogStatus()
		if err != nil {
			fatal(err)
		}
		if st == nil || !st.Active {
			fmt.Println("logging inactive")
			return
		}
		fmt.Printf("logging to %s (%s)\n", st.Path, st.Format)
	default:
		fatal(fmt.Errorf("unknown log subcommand %q", sub))
	}
}
