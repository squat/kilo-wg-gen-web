package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	kiloclient "github.com/squat/kilo/pkg/k8s/clientset/versioned"
	"gitlab.127-0-0-1.fr/vx3r/wg-gen-web/model"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	logLevelAll         = "all"
	logLevelDebug       = "debug"
	logLevelInfo        = "info"
	logLevelWarn        = "warn"
	logLevelError       = "error"
	logLevelNone        = "none"
	managedByLabelKey   = "app.kubernetes.io/managed-by"
	managedByLabelValue = "kilo-wg-gen-web"
)

var (
	availableLogLevels = strings.Join([]string{
		logLevelAll,
		logLevelDebug,
		logLevelInfo,
		logLevelWarn,
		logLevelError,
		logLevelNone,
	}, ", ")
)

func main() {
	cmd := &cobra.Command{
		Use:   "kilo-wg-gen-web",
		Args:  cobra.ArbitraryArgs,
		Short: "Use Kilo as a backend for Wg Gen Web",
		Long:  "",
	}
	var kubeconfig string
	cmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", os.Getenv("KUBECONFIG"), "Path to kubeconfig.")
	var dir string
	cmd.PersistentFlags().StringVar(&dir, "dir", os.Getenv("WG_CONF_DIR"), "Path to the Wg Gen Web configuration directory.")
	var listen string
	cmd.PersistentFlags().StringVar(&listen, "listen", ":1107", "The address at which to listen for health and metrics.")
	var logLevel string
	cmd.PersistentFlags().StringVar(&logLevel, "log-level", logLevelInfo, fmt.Sprintf("Log level to use. Possible values: %s", availableLogLevels))

	var c kubernetes.Interface
	var kc kiloclient.Interface
	var logger log.Logger
	cmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		logger = log.NewJSONLogger(log.NewSyncWriter(os.Stdout))
		switch logLevel {
		case logLevelAll:
			logger = level.NewFilter(logger, level.AllowAll())
		case logLevelDebug:
			logger = level.NewFilter(logger, level.AllowDebug())
		case logLevelInfo:
			logger = level.NewFilter(logger, level.AllowInfo())
		case logLevelWarn:
			logger = level.NewFilter(logger, level.AllowWarn())
		case logLevelError:
			logger = level.NewFilter(logger, level.AllowError())
		case logLevelNone:
			logger = level.NewFilter(logger, level.AllowNone())
		default:
			return fmt.Errorf("log level %v unknown; possible values are: %s", logLevel, availableLogLevels)
		}
		logger = log.With(logger, "ts", log.DefaultTimestampUTC)
		logger = log.With(logger, "caller", log.DefaultCaller)

		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return fmt.Errorf("create Kubernetes config: %w", err)
		}
		c = kubernetes.NewForConfigOrDie(config)
		kc = kiloclient.NewForConfigOrDie(config)

		return nil
	}
	cmd.RunE = runCmd(&dir, &kc, &listen, &logger)

	for _, subCmd := range []*cobra.Command{
		setNode(&dir, &c),
	} {
		cmd.AddCommand(subCmd)
	}

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func runCmd(dir *string, kc *kiloclient.Interface, listen *string, logger *log.Logger) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, args []string) error {
		r := prometheus.NewRegistry()
		r.MustRegister(
			prometheus.NewGoCollector(),
			prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		)
		cfg := new(model.Server)
		j, err := ioutil.ReadFile(path.Join(*dir, serverConfigurationName))
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("read configuration file %q: %w", serverConfigurationName, err)
		}
		if j != nil {
			if err := json.Unmarshal(j, cfg); err != nil {
				return fmt.Errorf("unmarshal configuration as JSON: %w", err)
			}
		}

		c := newController(*dir, *kc, *logger, r, cfg)

		var g run.Group
		{
			// Run the HTTP server.
			mux := http.NewServeMux()
			mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			mux.Handle("/metrics", promhttp.HandlerFor(r, promhttp.HandlerOpts{}))
			l, err := net.Listen("tcp", *listen)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", *listen, err)
			}

			g.Add(func() error {
				if err := http.Serve(l, mux); err != nil && err != http.ErrServerClosed {
					return fmt.Errorf("error: server exited unexpectedly: %w", err)
				}
				return nil
			}, func(error) {
				l.Close()
			})
		}

		{
			stop := make(chan struct{})
			g.Add(func() error {
				if err := c.run(stop); err != nil {
					return fmt.Errorf("controller quit unexpectedly: %w", err)
				}
				return nil
			}, func(error) {
				close(stop)
			})
		}

		return g.Run()
	}
}
