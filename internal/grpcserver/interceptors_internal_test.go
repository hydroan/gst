package grpcserver

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

// TestCallsAreCountedInTheMetrics pins that the calls show up in the
// standard grpc_server_* metrics on the default registry, the one the
// metrics endpoint serves: the handled counter, labeled with the service,
// the method, the call type and the code, and the handling-time histogram.
func TestCallsAreCountedInTheMetrics(t *testing.T) {
	reset(t)
	echo(nil, nil)
	conn := dial(t, start(t), nil)
	handled := func() int {
		labels := map[string]string{"grpc_service": "gst.test.Echo", "grpc_method": "Ping", "grpc_type": "unary", "grpc_code": "OK"}
		return int(metricSample(t, "grpc_server_handled_total", labels).GetCounter().GetValue())
	}
	timed := func() uint64 {
		labels := map[string]string{"grpc_service": "gst.test.Echo", "grpc_method": "Ping", "grpc_type": "unary"}
		return metricSample(t, "grpc_server_handling_seconds", labels).GetHistogram().GetSampleCount()
	}
	handledBefore, timedBefore := handled(), timed()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, call(ctx, conn, "Ping"))

	require.Equal(t, handledBefore+1, handled())
	require.Equal(t, timedBefore+1, timed())
}

// metricSample reads the sample of family carrying labels off the default
// registry, nil when there is none: the getters of a nil sample answer zero.
func metricSample(t *testing.T, family string, labels map[string]string) *dto.Metric {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() != family {
			continue
		}
	samples:
		for _, m := range f.GetMetric() {
			for _, pair := range m.GetLabel() {
				if want, ok := labels[pair.GetName()]; ok && want != pair.GetValue() {
					continue samples
				}
			}
			return m
		}
	}
	return nil
}
