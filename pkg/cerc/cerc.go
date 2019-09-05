package cerc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"golang.org/x/xerrors"
)

// Duration is a json-unmarshallable wrapper for time.Duration.
// See https://stackoverflow.com/questions/48050945/how-to-unmarshal-json-into-durations
type Duration time.Duration

// UnmarshalJSON unmarshales a duration using ParseDuration
func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	switch value := v.(type) {
	case float64:
		*d = Duration(time.Duration(value))
		return nil
	case string:
		tmp, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		*d = Duration(tmp)
		return nil
	default:
		return xerrors.New("invalid duration")
	}
}

// BasicAuth configures basic authentication for HTTP requests or endpoints
type BasicAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Pathway is the template for a full-circle test
type Pathway struct {
	Name           string     `json:"name"`
	Endpoint       string     `json:"endpoint"`
	Method         string     `json:"method"`
	Authentication *BasicAuth `json:"auth,omitempty"`
	Payload        string     `json:"payload,omitempty"`
	Timeouts       struct {
		Request  Duration `json:"request,omitempty"`
		Response Duration `json:"response,omitempty"`
	} `json:"timeouts,omitempty"`
	Period Duration `json:"duration,omitempty"`
}

var validMethods = []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace}

func (p *Pathway) validate() error {
	if p.Name == "" {
		return xerrors.Errorf("Name is missing")
	}
	if p.Endpoint == "" {
		return xerrors.Errorf("Endpoint is missing")
	}
	if _, err := url.Parse(p.Endpoint); err != nil {
		return xerrors.Errorf("Endpoint is invalid: %w", err)
	}

	var isValidMethod bool
	for _, validMethod := range validMethods {
		if p.Method == validMethod {
			isValidMethod = true
			break
		}
	}
	if !isValidMethod {
		return xerrors.Errorf("Method \"%s\" is invalid", p.Method)
	}

	return nil
}

// Options configures a Cerc service instance
type Options struct {
	HTTPS struct {
		Cert string `json:"crt,omitempty"`
		Key  string `json:"key,omitempty"`
	} `json:"https,omitempty"`
	Pathways            []Pathway  `json:"pathways"`
	Address             string     `json:"address"`
	DefaultPeriod       Duration   `json:"defaultPeriod"`
	Auth                *BasicAuth `json:"auth,omitempty"`
	ResponseURLTemplate string     `json:"responseURLTemplate,omitempty"`
}

// fillInDefaults completes the options by setting default values if needed
func (c *Options) fillInDefaults() {
	if c.ResponseURLTemplate == "" {
		c.ResponseURLTemplate = "{{ .Scheme }}://{{ .Address }}/callback/{{ .Name }}"
	}

	if c.DefaultPeriod == Duration(0*time.Second) {
		c.DefaultPeriod = Duration(30 * time.Second)
	}
	for i, pth := range c.Pathways {
		if pth.Period == Duration(0*time.Second) {
			c.Pathways[i].Period = c.DefaultPeriod
		}
		if pth.Timeouts.Request == Duration(0*time.Second) {
			c.Pathways[i].Timeouts.Request = Duration(5 * time.Second)
		}
		if pth.Timeouts.Response == Duration(0*time.Second) {
			c.Pathways[i].Timeouts.Response = pth.Period / Duration(2)
		}
	}
}

// validate ensures the options/configuration is valid
func (c *Options) validate() error {
	for _, pth := range c.Pathways {
		err := pth.validate()
		if err != nil {
			return xerrors.Errorf("pathway %s invalid: %w", pth.Name, err)
		}
	}

	if c.Address == "" {
		return xerrors.Errorf("Address is missing")
	}

	return nil
}

// runner actually probes/acts on a pathway
type runner struct {
	C *Cerc
	P Pathway

	mu     sync.Mutex
	active map[string]*probe
}

func (r *runner) Run() {
	r.active = make(map[string]*probe)

	ticker := time.NewTicker(time.Duration(r.P.Period))
	for {
		<-ticker.C

		tkn := uuid.NewV4().String()
		go r.probe(tkn)
	}
}

const (
	// HeaderURL is the HTTP sent along with Cerc requests which contains the URL where we expect the resonse to
	HeaderURL = "X-Cerc-URL"

	// HeaderToken is the HTTP sent along with Cerc requests which contains the token to authenticate the response as
	HeaderToken = "X-Cerc-Token"
)

func (r *runner) probe(tkn string) {
	responseURL, err := r.C.buildResponseURL(r.P.Name, tkn)
	if err != nil {
		log.WithField("pathway", r.P.Name).WithError(err).Warn("cannot build resposne URL")
		return
	}

	req, err := http.NewRequest(r.P.Method, r.P.Endpoint, strings.NewReader(r.P.Payload))
	req.Header.Add(HeaderToken, tkn)
	req.Header.Add(HeaderURL, responseURL.String())

	r.registerProbe(tkn)
	var client = &http.Client{
		Timeout: time.Duration(r.P.Timeouts.Request),
	}
	resp, err := client.Do(req)
	if err != nil {
		r.failProbeIfUnresolved(tkn, err.Error())
		return
	}
	if resp.StatusCode != http.StatusOK {
		r.failProbeIfUnresolved(tkn, fmt.Sprintf("expected 200 status code, got %d", resp.StatusCode))
		return
	}

	// give the others some time to respond to this probe
	time.Sleep(time.Duration(r.P.Timeouts.Response))
	r.failProbeIfUnresolved(tkn, "response timeout")
}

func (r *runner) registerProbe(tkn string) {
	r.mu.Lock()
	r.active[tkn] = &probe{Started: time.Now()}
	r.mu.Unlock()
}

func (r *runner) failProbeIfUnresolved(tkn, reason string) {
	r.mu.Lock()
	_, exists := r.active[tkn]
	if !exists {
		// probe was resolved earlier - we're done here
		r.mu.Unlock()
		return
	}

	delete(r.active, tkn)
	r.mu.Unlock()

	// TODO: tell someone about this
	log.WithField("pathway", r.P.Name).WithField("reason", reason).Warn("pathway probe failed")
}

func (r *runner) Answer(tkn string) (ok bool) {
	r.mu.Lock()

	p, ok := r.active[tkn]
	if !ok {
		r.mu.Unlock()
		return false
	}
	delete(r.active, tkn)
	r.mu.Unlock()

	// TODO: tell someone about our success here
	dur := time.Since(p.Started)
	log.WithField("pathway", r.P.Name).WithField("duration", dur).Info("circle complete")

	return true
}

// probe is an active measurement on a pathway
type probe struct {
	Started time.Time
}

// Cerc is the service itself - create with New()
type Cerc struct {
	Config Options

	runners map[string]*runner
	router  *http.ServeMux
}

// Start creates a new cerc instance after validating its configuration
func Start(cfg Options) (c *Cerc, err error) {
	cfg.fillInDefaults()
	err = cfg.validate()
	if err != nil {
		return nil, err
	}

	c = &Cerc{
		Config: cfg,
	}
	c.routes()
	go http.ListenAndServe(cfg.Address, c.router)
	c.run()

	return c, nil
}

func (c *Cerc) routes() {
	c.router = http.NewServeMux()

	c.router.Handle("/selftest/positive", &Receiver{})
	c.router.Handle("/selftest/resp-timeout", &Receiver{
		Responder: func(url, tkn string) error {
			go func() {
				time.Sleep(1 * time.Second)
				defaultResponder(url, tkn)
			}()
			return nil
		},
	})

	c.router.HandleFunc("/callback/", c.callback)
}

func (c *Cerc) run() {
	c.runners = make(map[string]*runner)
	for _, pth := range c.Config.Pathways {
		r := &runner{C: c, P: pth}
		c.runners[pth.Name] = r

		go r.Run()
	}
}

func (c *Cerc) callback(resp http.ResponseWriter, req *http.Request) {
	authUser, token, ok := req.BasicAuth()
	if !ok {
		resp.WriteHeader(http.StatusUnauthorized)
		return
	}
	if authUser != "Bearer" {
		resp.WriteHeader(http.StatusForbidden)
		return
	}

	name := strings.TrimPrefix(req.URL.Path, "/callback/")
	runner, ok := c.runners[name]
	if !ok {
		resp.WriteHeader(http.StatusNotFound)
		return
	}

	ok = runner.Answer(token)
	if !ok {
		resp.WriteHeader(http.StatusForbidden)
		return
	}

	resp.WriteHeader(http.StatusOK)
}

func (c *Cerc) buildResponseURL(name, tkn string) (url *url.URL, err error) {
	tpl, err := template.New("url").Parse(c.Config.ResponseURLTemplate)
	if err != nil {
		return nil, err
	}

	addr := c.Config.Address
	if strings.HasPrefix(addr, ":") {
		host, err := os.Hostname()
		if err != nil {
			return nil, err
		}

		addr = host + addr
	}
	scheme := "https"
	if c.Config.HTTPS.Cert == "" || c.Config.HTTPS.Key == "" {
		scheme = "http"
	}
	data := map[string]string{
		"Address": addr,
		"Name":    name,
		"Token":   tkn,
		"Scheme":  scheme,
	}

	var out bytes.Buffer
	err = tpl.Execute(&out, data)
	if err != nil {
		return nil, err
	}

	return url.Parse(out.String())
}
