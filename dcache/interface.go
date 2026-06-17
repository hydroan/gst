package dcache

type CacheMetricsProvider interface {
	Metrics() *localMetrics
}
