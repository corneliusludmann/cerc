package prometheus

import (
	"net/http"
	"strings"

	"github.com/32leaves/cerc/pkg/cerc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

// StartReporter starts a prometheus reporter
func StartReporter(addr string, pathways []cerc.Pathway) (*PromReporter, error) {
	r := &PromReporter{}

	err := r.registerMetrics(pathways)
	if err != nil {
		return nil, err
	}

	go func() {
		log.WithField("address", addr+"/metrics").Info("Prometheus reporter running")
		err := r.Start(addr, pathways)
		if err != nil {
			log.WithError(err).Error("cannot start Prometheus reporter")
		}
	}()

	return r, nil
}

// PromReporter reports cerc results to Prometheus
type PromReporter struct {
	metrics map[string]map[string]prometheus.Counter
}

// Start starts the Prometheus reporter and makes its endpoint available on $addr/metrics.
// This function does not return until the server crashes/is stopped or err's. It's a good idea
// to call this as a Go routine.
func (pr *PromReporter) Start(addr string, pathways []cerc.Pathway) error {
	err := pr.registerMetrics(pathways)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return http.ListenAndServe(addr, mux)
}

const (
	metricTotal      = "total"
	metricSuccess    = "success"
	metricFailure    = "failure"
	metricNonStarter = "nonstarter"
)

func (pr *PromReporter) registerMetrics(pathways []cerc.Pathway) error {
	if pr.metrics != nil {
		// we're already initialized - nothing to do here
		return nil
	}

	pr.metrics = make(map[string]map[string]prometheus.Counter)
	for _, pth := range pathways {
		pack := make(map[string]prometheus.Counter)
		name := strings.Replace(pth.Name, "-", "_", -1)

		for _, metric := range []string{metricTotal, metricSuccess, metricFailure, metricNonStarter} {
			pack[metric] = prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "cerc",
				Subsystem: name,
				Name:      metric,
			})
			err := prometheus.Register(pack[metric])
			if err != nil {
				return err
			}
		}

		pr.metrics[pth.Name] = pack
	}

	return nil
}

// getMetric attempts to access a previously registered metric. If it fails to do so, it'll log out and return false
func (pr *PromReporter) getMetric(pathway, metric string) (m prometheus.Counter, ok bool) {
	pack, ok := pr.metrics[pathway]
	if !ok {
		log.WithField("pathway", pathway).WithField("metric", metric).Warn("cannot find metric for pathway - we might be reporting incorrectly")
		return nil, false
	}

	m, ok = pack[metric]
	if !ok {
		log.WithField("pathway", pathway).WithField("metric", metric).Warn("cannot find metric for pathway - we might be reporting incorrectly")
		return nil, false
	}

	return m, true
}

// ProbeStarted is called when a new probe was started
func (pr *PromReporter) ProbeStarted(pathway string) {
	metric, ok := pr.getMetric(pathway, metricTotal)
	if !ok {
		return
	}

	metric.Inc()
}

// ProbeFinished is called when the probe has finished
func (pr *PromReporter) ProbeFinished(report cerc.Report) {
	var (
		metric prometheus.Counter
		ok     bool
	)
	switch report.Result {
	case cerc.ProbeSuccess:
		metric, ok = pr.getMetric(report.Pathway, metricSuccess)
	case cerc.ProbeFailure:
		metric, ok = pr.getMetric(report.Pathway, metricFailure)
	case cerc.ProbeNonStarter:
		metric, ok = pr.getMetric(report.Pathway, metricNonStarter)
	}
	if !ok {
		return
	}

	metric.Inc()
}
