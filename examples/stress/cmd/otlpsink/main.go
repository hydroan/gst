// A minimal OTLP/HTTP trace sink: it accepts every POST /v1/traces, counts
// the requests, the bytes and the spans it is sent, and answers 200, so a
// server exporting to it measures the export path without a collector's own
// work on the same machine. GET /stats prints the counters.
package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	tracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

var requests, bytesIn, spans, bad atomic.Int64

func main() {
	http.HandleFunc("/v1/traces", func(w http.ResponseWriter, r *http.Request) {
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			zr, err := gzip.NewReader(r.Body)
			if err != nil {
				bad.Add(1)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer zr.Close()
			body = zr
		}
		data, err := io.ReadAll(body)
		if err != nil {
			bad.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var req tracev1.ExportTraceServiceRequest
		if err := proto.Unmarshal(data, &req); err != nil {
			bad.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var n int64
		for _, rs := range req.GetResourceSpans() {
			for _, ss := range rs.GetScopeSpans() {
				n += int64(len(ss.GetSpans()))
			}
		}
		requests.Add(1)
		bytesIn.Add(int64(len(data)))
		spans.Add(n)
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mustMarshal(&tracev1.ExportTraceServiceResponse{}))
	})
	http.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "requests=%d bytes=%d spans=%d bad=%d\n", requests.Load(), bytesIn.Load(), spans.Load(), bad.Load())
	})
	http.HandleFunc("/reset", func(w http.ResponseWriter, _ *http.Request) {
		requests.Store(0)
		bytesIn.Store(0)
		spans.Store(0)
		bad.Store(0)
		fmt.Fprintln(w, "ok")
	})
	server := &http.Server{Addr: "127.0.0.1:14318", ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(server.ListenAndServe())
}

func mustMarshal(m proto.Message) []byte {
	b, err := proto.Marshal(m)
	if err != nil {
		panic(err)
	}
	return b
}
