package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"local/AmoebaStorage/internal/engine"
	"local/AmoebaStorage/internal/server"
)

type conf struct {
	Port                 int
	DataDir              string
	EnableSharding       bool
	ShutdownTimeout      time.Duration
	RetriesLimit         int
	VirtualVolumesCap    int
	VirtualVolumeSizeCap int64
	CleanUpCycleWindow   time.Duration

	// ghost conf
	// StreamBufferSize int // Future: tune streaming scratchpad buffer (e.g. 32KB, 64KB, 128KB)
}

const (
	defaultPort                 = 8000
	defaultDataDir              = "./data"
	defaultEnableSharding       = false
	defaultShutdownTimeout      = 10 * time.Second
	defaultRetriesLimit         = 3
	defaultVirtualVolumesCap    = 3
	defaultVirtualVolumeSizeCap = 1000
	defaultCleanUpCycleWindow   = 48 * time.Hour
)

func loadConf() (*conf, error) {
	cfg := &conf{}

	flag.IntVar(&cfg.Port, "port", defaultPort, "HTTP server port (1-65535)")
	flag.StringVar(&cfg.DataDir, "data-dir", defaultDataDir, "Directory path for object storage")
	flag.BoolVar(&cfg.EnableSharding, "enable-sharding", defaultEnableSharding, "Enable 2-tier SHA-256 directory sharding")
	flag.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", defaultShutdownTimeout, "Timeout for graceful server shutdown")
	flag.IntVar(&cfg.RetriesLimit, "retries-limit", defaultRetriesLimit, "Maximum retries for I/O operations")
	flag.IntVar(&cfg.VirtualVolumesCap, "virtual-volumes", defaultVirtualVolumesCap, "Number of virtual volumes to provision")
	flag.Int64Var(&cfg.VirtualVolumeSizeCap, "volume-size-mb", defaultVirtualVolumeSizeCap, "Capacity in MB per virtual volume")
	flag.DurationVar(&cfg.CleanUpCycleWindow, "cleanup-cycle-window", defaultCleanUpCycleWindow, "Window duration for cleaning up ephemeral data")

	flag.Parse()

	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("invalid port %d: must be between 1 and 65535", cfg.Port)
	}

	if cfg.DataDir == "" {
		return nil, fmt.Errorf("data-dir cannot be empty")
	}

	if cfg.RetriesLimit < 1 {
		return nil, fmt.Errorf("retries limit cannot be less than 1")
	}

	if cfg.VirtualVolumesCap < 1 {
		return nil, fmt.Errorf("virtual-volumes must be at least 1")
	}

	if cfg.VirtualVolumeSizeCap < 1 {
		return nil, fmt.Errorf("volume-size-mb must be at least 1")
	}

	if cfg.CleanUpCycleWindow < 1*time.Minute {
		return nil, fmt.Errorf("cleanup-cycle-window must be at least 1 minute")
	}

	return cfg, nil
}

func main() {
	cfg, err := loadConf()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	logger.Info(
		"amoeba configuration loaded",
		"port", cfg.Port,
		"data_dir", cfg.DataDir,
		"sharding", cfg.EnableSharding,
		"retries", cfg.RetriesLimit,
		"virtual_volumes", cfg.VirtualVolumesCap,
		"volume_size_mb", cfg.VirtualVolumeSizeCap,
		"cleanup_cycle_window", cfg.CleanUpCycleWindow,
		"shutdown_timeout", cfg.ShutdownTimeout,
	)

	eng, err := engine.New(cfg.DataDir, cfg.EnableSharding, cfg.RetriesLimit, cfg.VirtualVolumesCap, cfg.VirtualVolumeSizeCap)
	if err != nil {
		logger.Error("failed to initialize storage engine", "error", err)
		os.Exit(1)
	}
	logger.Info("storage engine initialized successfully")
	_ = eng

	srv := server.New(cfg.Port, logger)

	go func() {
		if err := srv.Start(); err != nil {
			logger.Error("Sever Fatal Error: ", "error", err)
			os.Exit(1)
		}
	}()

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	sig := <-shutdownSignal
	logger.Info("shutdown signal recieved, draining connection...", "signal: ", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}
}
