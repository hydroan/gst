package database_test

import (
	"testing"

	"github.com/hydroan/gst/internal/model"
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
			model.Register[*TestUser]()
			model.Register[*TestItem]()
			model.Register[*TestPlainItem]()
			model.Register[*TestUniqueItem]()
			model.Register[*TestConstrainedRecord]()
			model.Register[*TestConstrainedEntry]()
			model.Register[*TestRequiredRecord]()
			model.Register[*TestIndexerUniqueItem]()
			model.Register[*TestMixedUniqueItem]()
			model.Register[*TestAutoItem]()
			model.Register[*TestHookConfig]()
			model.Register[*TestHookBox]()
			model.Register[*TestCategory]()
			model.Register[*TestAggregateRecord]()
			model.Register[*TestCursorSnapshot]()
			model.Register[*TestDatedCursor]()
			model.Register[*TestDatedRecord]()
			model.Register[*TestRecordTag]()
			model.Register[*TestTagNote]()
			model.Register[*TestPayment]()
			model.Register[*TestRefund]()
			model.Register[*TestAccount]()
			model.Register[*TestMarkedRecord]()
			model.Register[*TestScoredRecord]()
			model.Register[*TestTypedRecord]()
			model.Register[*TestHolderRecord]()
			model.Register[*TestHeldEntry]()
			model.Register[*TestZeroValueRecord]()
		},
	})
}
