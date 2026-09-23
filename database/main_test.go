package database_test

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/testutil"
)

// TestMain runs the suite against the dialect under test — MySQL, the
// framework's primary dialect, unless a build tag names another (see
// testutil.DatabaseUnderTest); the Makefile test target repeats the package
// once per dialect. Every test in
// this package must either behave identically across dialects or branch on
// config.App.Database.Type where a per-dialect contract differs (the Upsert
// collision test is the pattern). A dialect broken by an open bug takes a
// t.Skip carrying the bug number, so the account stays greppable until the
// fix lands.
func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Database: testutil.DatabaseUnderTest(),
		Register: func() {
			modelregistry.Register[*TestUser]()
			modelregistry.Register[*TestItem]()
			modelregistry.Register[*TestPlainItem]()
			modelregistry.Register[*TestUniqueItem]()
			modelregistry.Register[*TestIndexerUniqueItem]()
			modelregistry.Register[*TestMixedUniqueItem]()
			modelregistry.Register[*TestAutoItem]()
			modelregistry.Register[*TestHookConfig]()
			modelregistry.Register[*TestHookGroup]()
			modelregistry.Register[*TestCategory]()
			modelregistry.Register[*TestAggregateRecord]()
			modelregistry.Register[*TestCursorSnapshot]()
			modelregistry.Register[*TestRecordTag]()
			modelregistry.Register[*TestTagNote]()
			modelregistry.Register[*TestPayment]()
			modelregistry.Register[*TestRefund]()
			modelregistry.Register[*TestAccount]()
			modelregistry.Register[*TestMarkedRecord]()
			modelregistry.Register[*TestScoredRecord]()
		},
	})
}
