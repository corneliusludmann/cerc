package httpendpoint

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/32leaves/cerc/pkg/cerc"
	"github.com/jeremywohl/flatten"
)

// Reporter holds cerc reports in memory and serves them via HTTP
type Reporter struct {
	reports map[string]cerc.Report
}

// NewReporter constructs a new Reporter
func NewReporter() *Reporter {
	r := Reporter{}
	r.reports = make(map[string]cerc.Report)
	return &r
}

// ProbeStarted is called when a new probe was started
func (reporter *Reporter) ProbeStarted(pathway string) {}

// ProbeFinished is called when the probe has finished
func (reporter *Reporter) ProbeFinished(report cerc.Report) {
	reporter.reports[report.Pathway] = report
}

// Serve provides reports via HTTP
func (reporter *Reporter) Serve(w http.ResponseWriter, r *http.Request) {

	var (
		msg []byte
		err error
	)

	format := r.FormValue("format")
	switch format {
	case "raw":
		msg, err = json.Marshal(reporter.reports)
	case "json_flat":
		msg, err = json.Marshal(reporter.summary(true))
	default:
		msg, err = json.Marshal(reporter.summary(false))
	}

	if err != nil {
		fmt.Fprintln(w, "unexpected error", err)
	} else {
		fmt.Fprintln(w, string(msg))
	}
}

func (reporter *Reporter) summary(flat bool) map[string]interface{} {
	result := make(map[string]interface{})
	result["status"] = "healthy"
	for _, r := range reporter.reports {
		if r.Result != cerc.ProbeSuccess {
			result["status"] = "unhealthy"
		}
		var message interface{}
		err := json.Unmarshal([]byte(r.Message), &message)
		if err != nil {
			message = r.Message
		}
		result[r.Pathway] = map[string]interface{}{
			"result":    r.Result,
			"message":   message,
			"timestamp": r.Timestamp,
		}
	}
	if flat {
		result, _ = flatten.Flatten(result, "", flatten.DotStyle)
		return result
	}
	return result
}
