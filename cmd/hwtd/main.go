// hwtd is the HW-T monitoring daemon: it discovers providers, polls
// sensors into the core registry, evaluates alerts, drives the sensor log,
// and serves the UDS API and /metrics.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/zen66ten/HW-T/internal/api/metrics"
	"github.com/zen66ten/HW-T/internal/api/uds"
	"github.com/zen66ten/HW-T/internal/core"
	"github.com/zen66ten/HW-T/internal/providers/amdgpu"
	"github.com/zen66ten/HW-T/internal/providers/cpu"
	"github.com/zen66ten/HW-T/internal/providers/dmi"
	"github.com/zen66ten/HW-T/internal/providers/edac"
	"github.com/zen66ten/HW-T/internal/providers/edid"
	"github.com/zen66ten/HW-T/internal/providers/hwmon"
	"github.com/zen66ten/HW-T/internal/providers/nvidia"
	"github.com/zen66ten/HW-T/internal/providers/pci"
	"github.com/zen66ten/HW-T/internal/providers/rapl"
	"github.com/zen66ten/HW-T/internal/providers/smart"
	"github.com/zen66ten/HW-T/internal/providers/usb"
)

type alertConfig struct {
	Name       string   `toml:"name"`
	Sensor     string   `toml:"sensor"`
	Above      *float64 `toml:"above"`
	Below      *float64 `toml:"below"`
	For        string   `toml:"for"`        // duration, e.g. "10s"
	Hysteresis float64  `toml:"hysteresis"` // same unit as the sensor
	Actions    []string `toml:"actions"`    // journal | notify | exec:<cmd> | webhook:<url>
}

type config struct {
	Daemon struct {
		Socket string `toml:"socket"`
		Listen string `toml:"listen"`
	} `toml:"daemon"`
	Poll struct {
		Fast   string `toml:"fast"`   // hwmon, cpu, rapl, amdgpu (default 1s)
		Medium string `toml:"medium"` // nvidia (default 2s)
		Slow   string `toml:"slow"`   // smart (default 60s)
	} `toml:"poll"`
	History struct {
		Duration string `toml:"duration"` // ring buffer span (default 2h)
	} `toml:"history"`
	Log struct {
		Path   string `toml:"path"`   // default log file for `hwtctl log start`
		Format string `toml:"format"` // csv (default) or ndjson
	} `toml:"log"`
	Alerts []alertConfig `toml:"alert"`
}

func defaultSocket() string {
	if os.Geteuid() == 0 {
		return "/run/hwtd.sock"
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir + "/hwtd.sock"
	}
	return "/tmp/hwtd.sock"
}

func main() {
	configPath := flag.String("config", "/etc/hw-t/config.toml", "config file (missing file is fine, defaults apply)")
	socket := flag.String("socket", "", "unix socket path (default /run/hwtd.sock as root, $XDG_RUNTIME_DIR/hwtd.sock otherwise)")
	listen := flag.String("listen", "", "HTTP listen address for /metrics (default 127.0.0.1:11988)")
	sysfs := flag.String("sysfs", "/sys", "sysfs mount point (point at a fixture tree for testing)")
	flag.Parse()

	if err := run(*configPath, *socket, *listen, *sysfs); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func parseInterval(s string, def time.Duration, what string) (time.Duration, error) {
	if s == "" {
		return def, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", what, err)
	}
	return d, nil
}

func run(configPath, socket, listen, sysfs string) error {
	var cfg config
	if raw, err := os.ReadFile(configPath); err == nil {
		if err := toml.Unmarshal(raw, &cfg); err != nil {
			return fmt.Errorf("parsing %s: %w", configPath, err)
		}
		slog.Info("config loaded", "path", configPath)
	}
	if socket == "" {
		socket = cfg.Daemon.Socket
	}
	if socket == "" {
		socket = defaultSocket()
	}
	if listen == "" {
		listen = cfg.Daemon.Listen
	}
	if listen == "" {
		listen = "127.0.0.1:11988"
	}

	fast, err := parseInterval(cfg.Poll.Fast, time.Second, "poll.fast")
	if err != nil {
		return err
	}
	medium, err := parseInterval(cfg.Poll.Medium, 2*time.Second, "poll.medium")
	if err != nil {
		return err
	}
	slow, err := parseInterval(cfg.Poll.Slow, 60*time.Second, "poll.slow")
	if err != nil {
		return err
	}
	history, err := parseInterval(cfg.History.Duration, 2*time.Hour, "history.duration")
	if err != nil {
		return err
	}

	rules, err := buildAlertRules(cfg.Alerts)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	reg := core.NewRegistry(int(history / fast))
	sched := core.NewScheduler(reg,
		hwmon.New(sysfs, fast),
		cpu.New(sysfs, fast),
		rapl.New(sysfs, fast),
		amdgpu.New(sysfs, fast),
		nvidia.New(medium),
		smart.New(sysfs, slow),
		pci.New(sysfs, slow),
		edac.New(sysfs, slow),
		dmi.New(sysfs),
		usb.New(sysfs),
		edid.New(sysfs),
	)
	sched.Start(ctx)

	logger := core.NewLogger()
	go func() {
		ticker := time.NewTicker(fast)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				logger.Tick(reg.Snapshot(), now)
			}
		}
	}()

	var alerts *core.AlertEngine
	if len(rules) > 0 {
		alerts = core.NewAlertEngine(reg, rules)
		go alerts.Run(ctx, fast)
		slog.Info("alert engine started", "rules", len(rules))
	}

	srv, err := uds.Listen(socket, reg)
	if err != nil {
		return fmt.Errorf("uds: %w", err)
	}
	srv.Logger = logger
	srv.Alerts = alerts
	srv.DefaultLogPath = cfg.Log.Path
	go func() {
		if err := srv.Serve(ctx); err != nil {
			slog.Error("uds server", "err", err)
		}
	}()
	slog.Info("uds api ready", "socket", socket)

	var alertsFn func() []core.AlertStatus
	if alerts != nil {
		alertsFn = alerts.Statuses
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.Handler(reg, alertsFn))
	httpSrv := &http.Server{Addr: listen, Handler: mux}
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server", "err", err)
		}
	}()
	slog.Info("metrics ready", "addr", listen+"/metrics")

	<-ctx.Done()
	slog.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	httpSrv.Shutdown(shutCtx)
	if active, _, _ := logger.Status(); active {
		logger.Stop()
	}
	os.Remove(socket)
	return nil
}

func buildAlertRules(configs []alertConfig) ([]core.AlertRule, error) {
	var rules []core.AlertRule
	for i, ac := range configs {
		if ac.Sensor == "" {
			return nil, fmt.Errorf("alert %d: sensor is required", i)
		}
		if ac.Above == nil && ac.Below == nil {
			return nil, fmt.Errorf("alert %q: needs above or below", ac.Name)
		}
		forD, err := parseInterval(ac.For, 0, fmt.Sprintf("alert %q for", ac.Name))
		if err != nil {
			return nil, err
		}
		actions := ac.Actions
		if len(actions) == 0 {
			actions = []string{"journal"}
		}
		rules = append(rules, core.AlertRule{
			Name:       ac.Name,
			Sensor:     core.SensorID(ac.Sensor),
			Above:      ac.Above,
			Below:      ac.Below,
			For:        forD,
			Hysteresis: ac.Hysteresis,
			Actions:    actions,
		})
	}
	return rules, nil
}
