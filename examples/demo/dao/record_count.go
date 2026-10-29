// Package dao holds what the services share below the API: here a figure a
// job keeps for the process and a service answers from without a query.
package dao

import "sync/atomic"

var recordCount atomic.Int64

// RecordCount returns how many records the table held when the count was
// last refreshed, a process-local figure a request answers without a query.
func RecordCount() int64 {
	return recordCount.Load()
}

// SetRecordCount records the count the last refresh found.
func SetRecordCount(total int64) {
	recordCount.Store(total)
}
