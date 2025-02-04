package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
)

const (
	metricPrefix = "liquidctl"

	envPort     = "LIQUIDCTL_EXPORTER_PORT"
	envInterval = "LIQUIDCTL_EXPORTER_INTERVAL"
	envPath     = "LIQUIDCTL_EXPORTER_PATH"
)

var (
	defaultPort      = "9530"
	defaultInterval  = "10"
	defaultLiquidCMD = "/usr/local/bin/liquidctl"
)

// liquidctl statistic object as of v1.6.x
type liquidctlStatistic struct {
	Bus         string   `json:"bus"`
	Address     string   `json:"address"`
	Description string   `json:"description"`
	Status      []status `json:"status"`
}

// liquidctl status
type status struct {
	Key   string  `json:"key"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

// Metrics store per device ({deviceID: {metricID: prom.Gauge}})
var devices = map[string]map[string]prometheus.Gauge{}

// path to liquidctl executable
var path string

func init() {
	p, ok := os.LookupEnv(envPath)
	if !ok {
		path = defaultLiquidCMD
	} else {
	        path = p
	}
	log.Infof("liquidctl configured path, %s", path)

	// Register metrics available for each liquidctl device
	for _, d := range liquidctl_stats() {
		dname := deviceName(d.Address)
		devices[dname] = map[string]prometheus.Gauge{}
		for _, m := range d.Status {
			name := metricName(m.Key, dname)
			log.Infof("Registering metric %s for %s device", name, dname)
			devices[dname][name] = prometheus.NewGauge(
				prometheus.GaugeOpts{
					Name: name,
					Help: fmt.Sprintf("%s %s (%s).", d.Description, m.Key, m.Unit),
				},
			)
			prometheus.MustRegister(devices[dname][name])
		}
	}
}

func main() {
	log.Info("Starting liquidctl exporter")

	port, ok := os.LookupEnv(envPort)
	if !ok {
		port = defaultPort
	}

	interval, ok := os.LookupEnv(envInterval)
	if !ok {
		interval = defaultInterval
	}
	i, _ := strconv.Atoi(interval)

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
			<head><title>liquidctl Exporter</title></head>
			<body>
			<h1>liquidctl Exporter</h1>
			<p><a href="/metrics">Metrics</a></p>
			</body>
			</html>`))
	})
	log.Infof("Exposing mertics over HTTP on port: %s", port)
	go http.ListenAndServe(fmt.Sprintf(":%s", port), nil)

	// collection loop, without Desc/Collector pathern
	for {
		for _, d := range liquidctl_stats() {
			dname := deviceName(d.Address)
			for _, m := range d.Status {
				name := metricName(m.Key, dname)
				devices[dname][name].Set(m.Value)
			}
		}

		time.Sleep(time.Second * time.Duration(i))
	}
}

func liquidctl_stats() []liquidctlStatistic {
	cmd := exec.Command(path, "status", "--json")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	var stats []liquidctlStatistic
	if err := json.Unmarshal([]byte(out.String()), &stats); err != nil {
		log.Fatal(err)
	}

	return stats
}

func metricName(n, device string) string {
	// Format: metricPrefix_deviceID_metric
	// replace spaces with underscores
	name := strings.ReplaceAll(n, " ", "_")
	// trim + signes
	name = strings.Trim(strings.ToLower(name), "+")
	// Append common prefix for all metrics
	name = fmt.Sprintf("%s_%s_%s", metricPrefix, device, name)
	// trimming dots by split+join
	return strings.Join(strings.Split(name, "."), "")
}

// returns address unchanged to work on TrueNAS since /dev/ is not used on FreeBSD
func deviceName(n string) string {
	return n
}
