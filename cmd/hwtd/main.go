// hwtd is the HW-T monitoring daemon: it discovers providers, polls
// sensors into the core registry, and serves the UDS API and /metrics.
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
	"github.com/zen66ten/HW-T/internal/providers/cpu"
	"github.com/zen66ten/HW-T/internal/providers/dmi"
	"github.com/zen66ten/HW-T/internal/providers/hwmon"
)

type config struct {
	Daemon struct {
		Socket string `toml:"socket"`
		Listen string `toml:"listen"`
	} `toml:"daemon"`
	Poll struct {
		Fast string `toml:"fast"` // e.g. "1s"
	} `toml:"poll"`
	History struct {
		Duration string `toml:"duration"` // e.g. "2h"
	} `toml:"history"`
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
	interval := time.Second
	if cfg.Poll.Fast != "" {
		d, err := time.ParseDuration(cfg.Poll.Fast)
		if err != nil {
			return fmt.Errorf("poll.fast: %w", err)
		}
		interval = d
	}
	history := 2 * time.Hour
	if cfg.History.Duration != "" {
		d, err := time.ParseDuration(cfg.History.Duration)
		if err != nil {
			return fmt.Errorf("history.duration: %w", err)
		}
		history = d
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	reg := core.NewRegistry(int(history / interval))
	sched := core.NewScheduler(reg, interval,
		hwmon.New(sysfs),
		cpu.New(sysfs),
		dmi.New(sysfs),
	)
	sched.Start(ctx)

	srv, err := uds.Listen(socket, reg)
	if err != nil {
		return fmt.Errorf("uds: %w", err)
	}
	go func() {
		if err := srv.Serve(ctx); err != nil {
			slog.Error("uds server", "err", err)
		}
	}()
	slog.Info("uds api ready", "socket", socket)

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.Handler(reg))
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
	os.Remove(socket)
	return nil
}
