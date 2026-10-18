package testcontainer

import (
	"context"
	"os"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/elasticsearch"
)

const (
	elasticsearchImage = "docker.elastic.co/elasticsearch/elasticsearch:8.19.23"

	// elasticsearchHeapOptions sizes the JVM. Elasticsearch reads
	// ES_JAVA_OPTS after its jvm.options files, so this overrides the two
	// gigabytes the testcontainers module writes there.
	elasticsearchHeapOptions = "-Xms512m -Xmx512m"
)

// SetupElasticsearch starts an Elasticsearch container of its own and points
// the framework at it, returning the function that terminates it.
//
// Like kafka's and rocketmq's, the container is dedicated even by default and
// left to testcontainers alone, the reaper included: an index would be a slot
// a shared container could hand out, but a resident Elasticsearch is one more
// JVM kept running for the tests of one package, which the start it saves
// does not buy.
//
// Security is switched off, so the framework reaches the cluster over plain
// HTTP without credentials and the fixture hands over an address alone; with
// security on, the module generates a certificate authority the client would
// have to be given as a file.
func SetupElasticsearch() (func() error, error) {
	muteContainerLog()
	ctx := context.Background()

	c, err := elasticsearch.Run(ctx, elasticsearchImage, testcontainers.WithEnv(map[string]string{
		"xpack.security.enabled": "false",
		"ES_JAVA_OPTS":           elasticsearchHeapOptions,
	}))
	if err != nil {
		err = errors.Wrap(err, "failed to start elasticsearch container")
		if c != nil {
			err = errors.CombineErrors(err, c.Terminate(ctx))
		}
		return nil, err
	}
	terminate := func() error { return c.Terminate(ctx) }

	// Addrs is a slice, which ApplyConfigToEnv skips; the variable carries
	// the comma-separated form the config layer splits back apart.
	addr := c.Settings.Address
	os.Setenv(config.ELASTICSEARCH_ADDRS, addr)
	os.Setenv(config.ELASTICSEARCH_ENABLED, "true")
	reportServiceReady("elasticsearch", addr)

	return terminate, nil
}
