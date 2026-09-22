package database_test

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/testutil"
)

// TestMain runs the suite against the dialect under test — MySQL, the
// framework's primary dialect, unless GST_TEST_DATABASE names another; the
// Makefile test target repeats the package once per dialect. Every test in
// this package must either behave identically across dialects or branch on
// config.App.Database.Type where a per-dialect contract differs (the Upsert
// collision test is the pattern). A dialect broken by an open bug takes a
// t.Skip carrying the bug number, so the account stays greppable until the
// fix lands.
func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Database: testutil.DatabaseUnderTest(),
		Register: func() {
			modelregistry.RegisterTable[*TestUser]()
			modelregistry.RegisterTable[*TestItem]()
			modelregistry.RegisterTable[*TestPlainItem]()
			modelregistry.RegisterTable[*TestUniqueItem]()
			modelregistry.RegisterTable[*TestIndexerUniqueItem]()
			modelregistry.RegisterTable[*TestMixedUniqueItem]()
			modelregistry.RegisterTable[*TestAutoItem]()
			modelregistry.RegisterTable[*TestHookConfig]()
			modelregistry.RegisterTable[*TestHookGroup]()
			modelregistry.RegisterTable[*TestCategory]()
			modelregistry.RegisterTable[*TestAggregateRecord]()
			modelregistry.RegisterTable[*TestCursorSnapshot]()
			modelregistry.RegisterTable[*TestRecordTag]()
			modelregistry.RegisterTable[*TestTagNote]()
			modelregistry.RegisterTable[*TestPayment]()
			modelregistry.RegisterTable[*TestRefund]()
			modelregistry.RegisterTable[*TestAccount]()
			modelregistry.RegisterTable[*TestMarkedRecord]()
			modelregistry.RegisterTable[*TestScoredRecord]()
		},
	})
}
