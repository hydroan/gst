package mysql

import (
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestBuildDSN(t *testing.T) {
	base := config.MySQL{Host: "127.0.0.1", Port: 3306, Database: "sample", Username: "root", Password: "secret"}
	prefix := "root:secret@tcp(127.0.0.1:3306)/sample?charset=utf8mb4&parseTime=True&loc=UTC&clientFoundRows=true&interpolateParams=true"

	t.Run("without timeouts", func(t *testing.T) {
		require.Equal(t, prefix, buildDSN(base))
	})

	t.Run("dial timeout", func(t *testing.T) {
		cfg := base
		cfg.DialTimeout = 10 * time.Second
		require.Equal(t, prefix+"&timeout=10s", buildDSN(cfg))
	})

	t.Run("all timeouts", func(t *testing.T) {
		cfg := base
		cfg.DialTimeout = 5 * time.Second
		cfg.ReadTimeout = 30 * time.Second
		cfg.WriteTimeout = time.Minute
		require.Equal(t, prefix+"&timeout=5s&readTimeout=30s&writeTimeout=1m0s", buildDSN(cfg))
	})
}
