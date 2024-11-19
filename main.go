// Package main provides a Discord bot that reads msgs from discord channels and stores them in PebbleDB
// which are then sent to a specified Discord channel. It includes functionality to handle
// message content and attachments, and ensures proper error handling and resource cleanup.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

var numberOfHTTPRequests = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "http_requests_total",
	Help: "Number of HTTP requests",
})

func init() {
	prometheus.MustRegister(numberOfHTTPRequests)
}

func main() {
	log := logr.FromSlogHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: false,
	}))

	conf := struct {
		addr        string
		metricsAddr string
		pprofAddr   string
		enablePprof bool
	}{
		addr:        ":3311",
		metricsAddr: ":3001",
		pprofAddr:   ":6060",
		enablePprof: true,
	}

	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "addr",
				EnvVars:     []string{"ADDR"},
				Destination: &conf.addr,
				Value:       conf.addr,
			},
			&cli.StringFlag{
				Name:        "metrics-addr",
				EnvVars:     []string{"METRICS_ADDR"},
				Destination: &conf.metricsAddr,
				Value:       conf.metricsAddr,
			},
			&cli.StringFlag{
				Name:        "pprof-addr",
				EnvVars:     []string{"PPROF_ADDR"},
				Destination: &conf.pprofAddr,
				Value:       conf.pprofAddr,
			},
			&cli.BoolFlag{
				Name:        "enable-pprof",
				EnvVars:     []string{"ENABLE_PPROF"},
				Destination: &conf.enablePprof,
				Value:       conf.enablePprof,
			},
		},
		Action: func(c *cli.Context) error {
			eg, ctx := errgroup.WithContext(c.Context)

			if conf.enablePprof {
				eg.Go(func() error {
					return runHTTP(ctx, log, conf.pprofAddr, "pprof", http.DefaultServeMux)
				})
			}

			eg.Go(func() error {
				mux := http.NewServeMux()
				mux.Handle("/metrics", promhttp.Handler())
				return runHTTP(ctx, log, conf.metricsAddr, "metrics", mux)
			})

			eg.Go(func() error {
				mux := http.NewServeMux()
				mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
					numberOfHTTPRequests.Inc()
					_, _ = w.Write([]byte("hello world"))
				})
				return runHTTP(ctx, log, conf.addr, "app", mux)
			})

			return eg.Wait()
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Error(err, "run")
		os.Exit(1)
	}
}

func runHTTP(ctx context.Context, log logr.Logger, addr, name string, handler http.Handler) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("could not listen for %s on address %w", name, err)

	}

	s := &http.Server{
		Handler: handler,
	}

	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		log.Info(fmt.Sprintf("initiated a graceful shutdown of the %s server", name))
		err := s.Shutdown(shutdownContext)
		if errors.Is(err, context.DeadlineExceeded) {
			log.Info(fmt.Sprintf("%s server terminated abruptly, necessitating a forced closure.", name))
			s.Close() // nolint: errcheck
		}
	}()

	log.Info(fmt.Sprintf("%s server is up and running", name), "addr", l.Addr().String())
	return s.Serve(l)
}
