// Package dao holds what the services share below the API: here a figure a
// job keeps for the process and a service answers from without a query.
package dao

import "sync/atomic"

var ticketCount atomic.Int64

// TicketCount returns how many tickets the table held when the count was
// last refreshed, a process-local figure a request answers without a query.
func TicketCount() int64 {
	return ticketCount.Load()
}

// SetTicketCount keeps the count the last refresh found.
func SetTicketCount(total int64) {
	ticketCount.Store(total)
}
