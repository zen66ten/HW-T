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
  sensors [-json]      current values with min/max/avg
  devices [-json]      discovered devices incl. DMI inventory
  history -id <id>     buffered samples for one sensor (JSON)
  reset [-id <id>]     reset min/max/avg (all sensors when -id omitted)
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
		if s.Err != "" {
			fmt.Printf("  %-24s%12s\n", s.Label, "N/A")
			continue
		}
		kind := core.Kind(s.Kind)
		fmt.Printf("  %-24s%12s%12s%12s%12s\n", s.Label,
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

func runReset(c *client.Client, args []string) {
	fs := flag.NewFlagSet("reset", flag.ExitOnError)
	id := fs.String("id", "", "sensor id (all when empty)")
	fs.Parse(args)
	if err := c.Reset(*id); err != nil {
		fatal(err)
	}
	fmt.Println("ok")
}
