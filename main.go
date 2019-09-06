/*
Copyright © 2019 Christian Weichel

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"github.com/32leaves/cerc/pkg/cerc"
	"github.com/32leaves/cerc/pkg/reporter/prometheus"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

type config struct {
	Service   cerc.Options `json:"service"`
	Reporting struct {
		Log        bool `json:"log,omitempty"`
		Prometheus bool `json:"prometheus,omitempty"`
	} `json:"reporting"`
	PProf bool `json:"pprof,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <config.json>", os.Args[0])
	}

	fc, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		log.WithError(err).Fatal("cannot read configuration")
	}

	var cfg config
	err = json.Unmarshal(fc, &cfg)
	if err != nil {
		log.WithError(err).Fatal("cannot read configuration")
	}

	mux := http.NewServeMux()
	go func() {
		err := http.ListenAndServe(cfg.Service.Address, mux)
		log.WithError(err).Fatal("cannot run service")
	}()

	var reporter []cerc.Reporter
	if cfg.Reporting.Log {
		reporter = append(reporter, loggingReporter{})
	}
	if cfg.Reporting.Prometheus {
		mux.Handle("/metrics", promhttp.Handler())

		rep, err := prometheus.StartReporter(cfg.Service.Pathways)
		if err != nil {
			log.WithError(err).Fatal("Prometheus reporter failed to start - exiting")
			return
		}
		reporter = append(reporter, rep)
	}

	c, err := cerc.Start(cfg.Service, cerc.NewCompositeReporter(reporter...), mux)
	if err != nil {
		log.WithError(err).Fatal("cannot start cerc service")
	}

	if cfg.PProf {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	}

	log.WithField("address", c.Config.Address).Info("cerc is up and running")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan
}

type loggingReporter struct{}

func (loggingReporter) ProbeStarted(pathway string) {}
func (loggingReporter) ProbeFinished(report cerc.Report) {
	switch report.Result {
	case cerc.ProbeSuccess:
		log.WithField("pathway", report.Pathway).WithField("duration", report.Duration).Info("circle complete")
	case cerc.ProbeFailure:
		log.WithField("pathway", report.Pathway).WithField("duration", report.Duration).WithField("reason", report.Message).Warn("pathway probe failed")
	case cerc.ProbeNonStarter:
		log.WithField("pathway", report.Pathway).WithField("reason", report.Message).Warn("pathway probe failed to start")
	}
}
