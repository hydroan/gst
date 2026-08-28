package clickhouse

import (
	"context"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/testcontainer"
)

func TestClickhouse(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		old := config.App.Clickhouse
		config.App.Clickhouse = config.Clickhouse{Enabled: false}
		t.Cleanup(func() { config.App.Clickhouse = old })

		if err := initProvider(); err != nil {
			t.Fatalf("Init with clickhouse disabled: %v", err)
		}
		if _, err := Client(); err == nil {
			t.Fatal("expected error from Client before initialization, got nil")
		}
	})

	t.Run("initialized against container", func(t *testing.T) {
		cfg, terminate, err := testcontainer.SetupClickhouse()
		if err != nil {
			t.Fatalf("start clickhouse container: %v", err)
		}
		t.Cleanup(func() { _ = terminate() })

		old := config.App.Clickhouse
		cfg.Compress = true
		config.App.Clickhouse = cfg
		t.Cleanup(func() {
			_ = closeProvider()
			config.App.Clickhouse = old
		})

		if err = initProvider(); err != nil {
			t.Fatalf("Init: %v", err)
		}

		c, err := Client()
		if err != nil {
			t.Fatalf("Client: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err = c.Exec(ctx, "CREATE TABLE samples (id UInt32, name String) ENGINE = Memory"); err != nil {
			t.Fatalf("create table: %v", err)
		}

		batch, err := c.PrepareBatch(ctx, "INSERT INTO samples")
		if err != nil {
			t.Fatalf("PrepareBatch: %v", err)
		}
		for i, name := range []string{"alpha", "beta", "gamma"} {
			if err = batch.Append(uint32(i+1), name); err != nil {
				t.Fatalf("Append: %v", err)
			}
		}
		if err = batch.Send(); err != nil {
			t.Fatalf("Send: %v", err)
		}

		var count uint64
		if err = c.QueryRow(ctx, "SELECT count() FROM samples").Scan(&count); err != nil {
			t.Fatalf("QueryRow: %v", err)
		}
		if count != 3 {
			t.Fatalf("expected 3 rows, got %d", count)
		}

		if err = closeProvider(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if _, err = Client(); err == nil {
			t.Fatal("expected error from Client after Close, got nil")
		}
	})
}
