package rpcerror

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/statsd/datadog"
)

// Logger middleware.
type Middleware struct {
	h     http.Handler
	stats *datadog.Client
}

// New logger middleware.
func New(stats *datadog.Client) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return &Middleware{
			h:     h,
			stats: stats,
		}
	}
}

// wrapper to capture status.
type wrapper struct {
	http.ResponseWriter
	written int
	status  int
}

// capture status.
func (w *wrapper) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// capture written bytes.
func (w *wrapper) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.written += n
	return n, err
}

// ServeHTTP implementation.
func (m *Middleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec := httptest.NewRecorder()

	b, _ := ioutil.ReadAll(r.Body)
	r.Body = ioutil.NopCloser(bytes.NewBuffer(b))

	m.h.ServeHTTP(rec, r)

	var req map[string]interface{}
	var res map[string]interface{}
	var param string

	err := json.Unmarshal(b, &req)
	if err != nil {
		fmt.Printf("rpcerror error: %s\n", err)
	}

	param = req["method"].(string)

	// copy original headers
	for k, v := range rec.Header() {
		w.Header()[k] = v
	}

	// copy status code
	w.WriteHeader(rec.Code)

	w.Write(rec.Body.Bytes())

	b = rec.Body.Bytes()
	err = json.Unmarshal(b, &res)
	if err != nil {
		fmt.Printf("rpcerror error: %s\n", err)
	}

	if msg := res["error"]; msg != nil {
		if msg == "not found" {
			m.stats.Incr("error.notfound", resource(param), method(param))
		} else {
			m.stats.Incr("error", resource(param), method(param))
		}
	} else {
		m.stats.Incr("query", resource(param), method(param))
	}
}

func resource(m string) string {
	return fmt.Sprintf("resource:%s", strings.ToLower(strings.Split(m, ".")[0]))
}

func method(m string) string {
	return fmt.Sprintf("method:%s", strings.ToLower(strings.Split(m, ".")[1]))
}
