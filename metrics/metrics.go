// Package prommetrics provides Prometheus metrics for the application; the name
// avoids conflicting with standard library or common "metrics" package names.
package prommetrics

import (
	"github.com/cockroachdb/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"go.uber.org/multierr"
)

const (
	NAMESPACE = "gst_"
	SUBSYSTEM = "backend_"
)

var (
	State                 prometheus.Gauge
	Uptime                prometheus.Gauge
	HTTPRequestsTotal     *prometheus.CounterVec
	HTTPRequestDuration   *prometheus.HistogramVec
	ResponseTime          prometheus.Histogram
	ErrorRate             prometheus.Counter
	MemoryTotal           prometheus.Gauge
	MemoryUsed            prometheus.Gauge
	MemoryUsedPercent     prometheus.Gauge
	CPUCount              prometheus.Gauge
	CPUUsedPercent        prometheus.Gauge
	ConcurrentConnections prometheus.Gauge
	DBConnectionsOpen     prometheus.Gauge
	CacheHit              *prometheus.CounterVec
	CacheMiss             *prometheus.CounterVec
	QueueSize             prometheus.Gauge
	AuthzPolicyDiverged   prometheus.Gauge
	AuthzDecisionsTotal   *prometheus.CounterVec
)

func Init() error {
	State = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: NAMESPACE,
		Subsystem: SUBSYSTEM,
		Name:      "state",
		Help:      "The state of the backend",
	})
	Uptime = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: NAMESPACE,
		Subsystem: SUBSYSTEM,
		Name:      "uptime",
		Help:      "The uptime of the backend",
	})
	// HttpRequestsTotal.WithLabelValues("GET").Inc()
	// HttpRequestsTotal.WithLabelValues("POST").Inc()
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: NAMESPACE,
			Subsystem: SUBSYSTEM,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: NAMESPACE,
			Subsystem: SUBSYSTEM,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latencies in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	ResponseTime = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: NAMESPACE,
		Subsystem: SUBSYSTEM,
		Name:      "response_time_seconds",
		Help:      "Response time in seconds",
		Buckets:   prometheus.DefBuckets,
	})
	ErrorRate = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: NAMESPACE,
		Subsystem: SUBSYSTEM,
		Name:      "error_total",
		Help:      "Total number of errors",
	})
	MemoryTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: NAMESPACE,
		Subsystem: SUBSYSTEM,
		Name:      "memory_usage_total_bytes",
		Help:      "Current memory total in bytes",
	})
	MemoryUsed = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: NAMESPACE,
		Subsystem: SUBSYSTEM,
		Name:      "memory_usage_used_bytes",
		Help:      "Current memory used in bytes",
	})
	MemoryUsedPercent = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: NAMESPACE,
		Subsystem: SUBSYSTEM,
		Name:      "memory_usage_percent",
		Help:      "Current memory used in percent",
	})
	CPUCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: NAMESPACE,
		Subsystem: SUBSYSTEM,
		Name:      "cpu_count",
		Help:      "Current cpu count",
	})
	CPUUsedPercent = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: NAMESPACE,
		Subsystem: SUBSYSTEM,
		Name:      "cpu_used_percent",
		Help:      "Current cpu used in percent",
	})
	ConcurrentConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: NAMESPACE,
		Subsystem: SUBSYSTEM,
		Name:      "concurrent_connections",
		Help:      "Number of concurrent connections",
	})
	DBConnectionsOpen = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: NAMESPACE,
		Subsystem: SUBSYSTEM,
		Name:      "db_connections_open",
		Help:      "Number of open database connections",
	})
	CacheHit = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: NAMESPACE,
		Subsystem: SUBSYSTEM,
		Name:      "cache_hits_total",
		Help:      "Total number of cache hits",
	}, []string{"phase", "table"})
	CacheMiss = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: NAMESPACE,
		Subsystem: SUBSYSTEM,
		Name:      "cache_misses_total",
		Help:      "Total number of cache misses",
	}, []string{"phase", "table"})
	QueueSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: NAMESPACE,
		Subsystem: SUBSYSTEM,
		Name:      "queue_size",
		Help:      "Current size of the task queue",
	})

	// A process in this state serves authorization decisions that disagree with
	// what is stored, and looks healthy from everywhere else: the write it lost
	// is already durable, the request that made it has returned, and comparing
	// stored rules against the records they come from cannot see a
	// disagreement that exists only in one process's memory.
	AuthzPolicyDiverged = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: NAMESPACE,
		Subsystem: SUBSYSTEM,
		Name:      "authz_policy_diverged",
		Help:      "Whether the in-memory authorization policy set no longer agrees with storage",
	})

	// Labeled by outcome and by the explanation that outcome can carry, all of
	// which are bounded by the model rather than by traffic: a decision is
	// allowed, denied or undecidable; an allowed one names one of a handful of
	// rule kinds; a denied one names which of the two steps behind a grant is
	// missing. Only the label belonging to the effect is filled, so the two
	// never multiply into series that cannot occur. Neither the tenant nor the
	// subject is a label — those grow with the deployment and belong in the
	// authz log, which carries them already.
	AuthzDecisionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: NAMESPACE,
		Subsystem: SUBSYSTEM,
		Name:      "authz_decisions_total",
		Help:      "Total authorization decisions by outcome, granting rule kind and denial reason",
	}, []string{"effect", "allowed_by", "denied_by"})

	errs := make([]error, 0, 19)
	errs = append(errs, prometheus.Register(State))
	errs = append(errs, prometheus.Register(Uptime))
	errs = append(errs, prometheus.Register(HTTPRequestsTotal))
	errs = append(errs, prometheus.Register(ResponseTime))
	errs = append(errs, prometheus.Register(ErrorRate))
	errs = append(errs, prometheus.Register(MemoryTotal))
	errs = append(errs, prometheus.Register(MemoryUsed))
	errs = append(errs, prometheus.Register(MemoryUsedPercent))
	errs = append(errs, prometheus.Register(CPUCount))
	errs = append(errs, prometheus.Register(CPUUsedPercent))
	errs = append(errs, prometheus.Register(ConcurrentConnections))
	errs = append(errs, prometheus.Register(DBConnectionsOpen))
	errs = append(errs, prometheus.Register(CacheHit))
	errs = append(errs, prometheus.Register(CacheMiss))
	errs = append(errs, prometheus.Register(QueueSize))
	errs = append(errs, prometheus.Register(AuthzPolicyDiverged))
	errs = append(errs, prometheus.Register(AuthzDecisionsTotal))

	errs = append(errs, prometheus.Register(collectors.NewBuildInfoCollector()))
	errs = append(errs, prometheus.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{Namespace: NAMESPACE})))
	// errs = append(errs, prometheus.Register(collectors.NewGoCollector()))
	// errs = append(errs, prometheus.Register(collectors.NewGoCollector(
	// 	collectors.WithGoCollections(collectors.GoRuntimeMetricsCollection),
	// 	collectors.WithGoCollections(collectors.GoRuntimeMemStatsCollection),
	// )))
	return errors.WithStack(multierr.Combine(errs...))
}
