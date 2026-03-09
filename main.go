package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	flags "github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

var defaultNvmlLibraryPaths = []string{
	"libnvidia-ml.so.1",
	"/usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1",
	"/usr/lib/aarch64-linux-gnu/libnvidia-ml.so.1",
	"/usr/lib64/libnvidia-ml.so.1",
	"/usr/lib/libnvidia-ml.so.1",
	// GKE / COS nodes
	"/home/kubernetes/bin/nvidia/lib64/libnvidia-ml.so.1",
}

func nvmlLibraryPaths() []string {
	extra := os.Getenv("NVML_LIBRARY_PATH")
	if extra == "" {
		return defaultNvmlLibraryPaths
	}
	paths := strings.Split(extra, ":")
	return append(paths, defaultNvmlLibraryPaths...)
}

type Options struct {
	ListenAddress string `short:"l" long:"listen-address" default:":8082" description:"Address to listen on for metrics"`
	MetricsPath   string `short:"m" long:"metrics-path" default:"/metrics" description:"Path under which to expose metrics"`
	LogLevel      string `long:"log-level" default:"info" choice:"debug" choice:"info" choice:"warn" choice:"error" description:"Log level"`
}

func main() {
	var opts Options
	parser := flags.NewParser(&opts, flags.Default)
	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		os.Exit(1)
	}

	logger := newLogger(opts.LogLevel)

	ret := initNVML(logger)
	if ret != nvml.SUCCESS {
		logger.Fatal().Str("error", nvml.ErrorString(ret)).Msg("failed to initialize NVML")
	}
	defer nvml.Shutdown()

	driverVersion, ret := nvml.SystemGetDriverVersion()
	if ret == nvml.SUCCESS {
		logger.Info().Str("driver_version", driverVersion).Msg("NVML initialized")
	}

	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		logger.Fatal().Str("error", nvml.ErrorString(ret)).Msg("failed to get device count")
	}
	logger.Info().Int("count", count).Msg("found GPUs")

	collector := NewNvidiaCollector(logger)
	prometheus.MustRegister(collector)

	mux := http.NewServeMux()
	mux.Handle(opts.MetricsPath, promhttp.Handler())
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<html><body><h1>NVIDIA GPU Exporter</h1><p><a href="%s">Metrics</a></p></body></html>`, opts.MetricsPath)
	})

	server := &http.Server{
		Addr:         opts.ListenAddress,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info().Str("address", opts.ListenAddress).Str("metrics_path", opts.MetricsPath).Msg("starting server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("server failed")
		}
	}()

	<-ctx.Done()
	logger.Info().Msg("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}

func initNVML(logger zerolog.Logger) nvml.Return {
	for _, path := range nvmlLibraryPaths() {
		if err := nvml.SetLibraryOptions(nvml.WithLibraryPath(path)); err != nil {
			logger.Debug().Str("path", path).Err(err).Msg("failed to set library path")
			continue
		}
		ret := nvml.Init()
		if ret == nvml.SUCCESS {
			logger.Info().Str("library", path).Msg("loaded NVML")
			return ret
		}
		logger.Debug().Str("path", path).Str("error", nvml.ErrorString(ret)).Msg("NVML init failed, trying next path")
	}
	return nvml.ERROR_LIBRARY_NOT_FOUND
}

func newLogger(logLevel string) zerolog.Logger {
	var level zerolog.Level
	switch logLevel {
	case "debug":
		level = zerolog.DebugLevel
	case "warn":
		level = zerolog.WarnLevel
	case "error":
		level = zerolog.ErrorLevel
	default:
		level = zerolog.InfoLevel
	}
	return zerolog.New(os.Stderr).
		Level(level).
		With().Timestamp().Logger()
}
