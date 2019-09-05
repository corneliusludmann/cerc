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
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/32leaves/cerc/pkg/cerc"

	log "github.com/sirupsen/logrus"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <config.json>", os.Args[0])
	}

	fc, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		log.WithError(err).Fatal("cannot read configuration")
	}

	var cfg cerc.Options
	err = json.Unmarshal(fc, &cfg)
	if err != nil {
		log.WithError(err).Fatal("cannot read configuration")
	}

	c, err := cerc.Start(cfg, loggingReporter{})
	if err != nil {
		log.WithError(err).Fatal("cannot start cerc service")
	}

	log.WithField("address", c.Config.Address).Info("cerc is up and running")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan
}

type loggingReporter struct{}

func (loggingReporter) Success(pathway string, dur time.Duration) {
	log.WithField("pathway", pathway).WithField("duration", dur).Info("circle complete")
}

func (loggingReporter) Failed(pathway, reason string) {
	log.WithField("pathway", pathway).WithField("reason", reason).Warn("pathway probe failed")
}

func (loggingReporter) NonStarter(pathway, reason string) {
	log.WithField("pathway", pathway).WithField("reason", reason).Warn("pathway probe failed to start")
}
