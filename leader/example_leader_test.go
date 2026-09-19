package leader_test

import (
	"context"
	"net"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/leader"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/provider/kafka"
	"github.com/twmb/franz-go/pkg/kgo"
)

// ExampleRegister registers leader work the way a project does, from the init
// function of its leader package. Every replica campaigns for the name; the
// one that wins runs the loop until ctx ends — the process begins shutting
// down, or the lease behind the leadership is lost — and another replica
// takes the name over: within about 6 seconds of a leader that shut down,
// within about 21 of one that crashed.
//
// Each name is campaigned for on its own: two names may be led by two
// replicas, and nothing tells other code whether this replica leads. Work
// that comes round on a schedule belongs to cronjob, which already runs each
// instant on one replica. Registration happens once, in init: a registration
// made after the elector started panics, and one with no name, a nil
// function or a name already registered fails the process at startup.
func ExampleRegister() {
	// leader/leader.go:
	//
	//	func init() {
	//		leader.Register(followUpstream, "upstream-follower")
	//	}
	leader.Register(func(ctx context.Context) error {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				// The tenure ended. Returning its ending is how a tenure ends:
				// logged as a step-down with its reason, not as a failure.
				return ctx.Err()
			case <-ticker.C:
				if err := pollUpstream(ctx); err != nil && ctx.Err() == nil {
					logger.App.Warnw("polling upstream failed", "err", err)
				}
			}
		}
	}, "upstream-follower")
}

// ExampleRegister_outboxRelay forwards the records waiting in an outbox table.
// The progress lives in the database, so the replica that takes the name over
// picks up where the last leader stopped.
//
// The mark that a record went out is written in a database.Transaction opened
// on the tenure's context: once the lease is lost, the transaction refuses to
// run before its first statement, so a replica that lost the name cannot mark
// what the new leader is handling. A write outside such a transaction is not
// checked against the lease, and neither is the send, a call on the wire: the
// loop looks at ctx before each record, and the receiver deduplicates by
// record ID, since a record sent just before the lease was lost is sent again
// by the new leader. On SQLite, where the framework opens a single connection,
// keep each transaction under 8 seconds, and under 5 when they run back to
// back: a longer one holds the lease renewal back until the lease counts as
// lost.
func ExampleRegister_outboxRelay() {
	leader.Register(func(ctx context.Context) error {
		for {
			records, err := pendingOutboxRecords(ctx, 100)
			if err != nil && ctx.Err() == nil {
				logger.App.Warnw("reading the outbox failed", "err", err)
			}
			for _, record := range records {
				if ctx.Err() != nil {
					break
				}
				if err := sendOutboxRecord(ctx, record); err != nil {
					logger.App.Warnw("sending an outbox record failed", "err", err, "record", record.ID)
					break
				}
				err := database.Transaction(ctx, func(ctx context.Context) error {
					return markOutboxRecordSent(ctx, record.ID)
				})
				if err != nil {
					logger.App.Warnw("marking an outbox record sent failed", "err", err, "record", record.ID)
					break
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		}
	}, "outbox-relay")
}

// ExampleRegister_resume carries a long scan across leaders. The work starts
// from scratch on every tenure — nothing it keeps in memory survives a change
// of leader — so it reads where the scan stands from the database first, and
// writes each batch's results together with the new position in one
// transaction: a tenure cut short anywhere loses at most the batch in flight,
// which the next leader scans again.
func ExampleRegister_resume() {
	leader.Register(func(ctx context.Context) error {
		cursor, err := loadScanCursor(ctx)
		if err != nil {
			return errors.Wrap(err, "load scan cursor")
		}
		for {
			batch, next, err := scanRecordsAfter(ctx, cursor, 500)
			if err != nil {
				return errors.Wrap(err, "scan records")
			}
			if len(batch) == 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(10 * time.Second):
				}
				continue
			}
			err = database.Transaction(ctx, func(ctx context.Context) error {
				return saveScanBatch(ctx, batch, next)
			})
			if err != nil {
				return errors.Wrap(err, "save scan batch")
			}
			cursor = next
		}
	}, "record-scan")
}

// ExampleRegister_kafkaConsumer has one replica of the deployment consume a
// Kafka topic, and another take over when it goes: within about 6 seconds of
// a leader that shut down, within about 21 of one that crashed or lost the
// primary database.
//
// The lease is what makes the consumer the only one, so the work assigns the
// partitions itself instead of joining a consumer group: a group would be a
// second coordinator, and the membership of a leader that crashed would hold
// the partitions until the group's session timeout — 45 seconds by default —
// well past the lease's.
//
// Where the consumption stands lives in the database, written together with
// each record's results in one database.Transaction opened on the tenure's
// context. The transaction refuses to run once the lease is lost, so a replica
// that lost the name cannot record progress in place of the new leader, and
// the new leader reads the offsets back and goes on from them: no record is
// skipped and none is applied twice. Offsets committed to Kafka instead would
// be a write the lease does not check: a record the last leader applied but
// had not committed yet would be applied again.
//
// The framework knows whether a replica reaches the primary database, not
// whether it reaches Kafka, and a poll does not fail while Kafka is out of
// reach, it just brings nothing. So a poll waits 30 seconds at most, and one
// that brought nothing pings Kafka: a replica that cannot reach it returns,
// which hands the name back, and waits a campaign interval before it
// campaigns again, so another replica usually takes the name over. Calls the
// work makes outside the transaction — a message produced to another topic, a
// call to another system — are not checked against the lease: their receivers
// deduplicate.
func ExampleRegister_kafkaConsumer() {
	leader.Register(func(ctx context.Context) error {
		offsets, err := loadConsumedOffsets(ctx, "events")
		if err != nil {
			return errors.Wrap(err, "load consumed offsets")
		}
		client, err := kafka.New(config.App.Kafka, kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{"events": offsets}))
		if err != nil {
			return errors.Wrap(err, "create kafka client")
		}
		defer client.Close()

		for {
			pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			fetches := client.PollFetches(pollCtx)
			cancel()
			if ctx.Err() != nil {
				// The tenure ended: stop consuming at once.
				return ctx.Err()
			}
			for _, fetchErr := range fetches.Errors() {
				// The end of the poll's own wait is not a failure.
				if !errors.Is(fetchErr.Err, context.DeadlineExceeded) {
					return errors.Wrapf(fetchErr.Err, "fetch %s partition %d", fetchErr.Topic, fetchErr.Partition)
				}
			}
			if fetches.NumRecords() == 0 {
				// A quiet topic, or Kafka out of reach: only a ping tells.
				if err := client.Ping(ctx); err != nil && ctx.Err() == nil {
					return errors.Wrap(err, "reach kafka")
				}
				continue
			}
			for _, record := range fetches.Records() {
				err := database.Transaction(ctx, func(ctx context.Context) error {
					if err := applyEvent(ctx, record.Value); err != nil {
						return err
					}
					return saveConsumedOffset(ctx, record.Topic, record.Partition, record.Offset+1)
				})
				if err != nil {
					return errors.Wrapf(err, "apply the event at %s partition %d offset %d", record.Topic, record.Partition, record.Offset)
				}
			}
		}
	}, "event-consumer")
}

// ExampleRegister_returning shows what each way out of the work means. The
// work is expected to run until ctx ends, and returning that ending — ctx.Err()
// or context.Cause(ctx), wrapped or not — ends the tenure normally. Returning
// before ctx ends, done or failed, hands the name back: the campaign resumes a
// few seconds later and the work runs again from scratch, on this replica or
// another; a returned failure is logged, and so is a panic, which is
// recovered. A transient failure is better retried in place, keeping the name.
//
// A failure of the work's own that meets the ending is reported only joined
// with it by errors.Join. Attached by errors.CombineErrors or formatted in with
// %v it goes unseen, and the tenure reads as ended normally.
func ExampleRegister_returning() {
	leader.Register(func(ctx context.Context) error {
		for {
			err := relayOnce(ctx)
			switch {
			case ctx.Err() != nil:
				// The tenure ended; a failure of the relay's own stays reported.
				return errors.Join(err, ctx.Err())
			case errors.Is(err, errRelayMisconfigured):
				// No retry fixes this here: hand the name back.
				return err
			case err != nil:
				logger.App.Warnw("relaying failed, retrying", "err", err)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		}
	}, "relay")
}

// ExampleRegister_singleConnection holds the one session an upstream allows
// for the whole deployment. The leader dials when its tenure starts and closes
// the session the moment the tenure ends — which is also what unblocks a read
// that takes no context. The work must return in time: 5 seconds after the
// lease was lost, work still running would be running beside the new leader's
// session, and the process fails and exits without waiting for it.
func ExampleRegister_singleConnection() {
	leader.Register(func(ctx context.Context) error {
		var dialer net.Dialer
		conn, err := dialer.DialContext(ctx, "tcp", "upstream.example:7000")
		if err != nil {
			return errors.Wrap(err, "dial upstream")
		}
		defer func() { _ = conn.Close() }()
		stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
		defer stop()

		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				return errors.Wrap(err, "read upstream session")
			}
			if err := handleUpstreamMessage(ctx, buf[:n]); err != nil && ctx.Err() == nil {
				logger.App.Warnw("handling an upstream message failed", "err", err)
			}
		}
	}, "upstream-session")
}

// The declarations below stand in for the project's own code.

// outboxRecord is one record waiting in the outbox.
type outboxRecord struct {
	ID int64
}

// errRelayMisconfigured reports a relay whose configuration cannot work.
var errRelayMisconfigured = errors.New("relay misconfigured")

// pollUpstream reads what changed upstream.
func pollUpstream(context.Context) error { return nil }

// pendingOutboxRecords reads up to limit records not sent yet, oldest first.
func pendingOutboxRecords(context.Context, int) ([]outboxRecord, error) { return nil, nil }

// sendOutboxRecord sends one record, with its ID for the receiver to
// deduplicate by.
func sendOutboxRecord(context.Context, outboxRecord) error { return nil }

// markOutboxRecordSent marks a record sent.
func markOutboxRecordSent(context.Context, int64) error { return nil }

// loadScanCursor reads the position the scan reached.
func loadScanCursor(context.Context) (int64, error) { return 0, nil }

// scanRecordsAfter reads up to limit record IDs past cursor, and the cursor
// after them.
func scanRecordsAfter(context.Context, int64, int) ([]int64, int64, error) { return nil, 0, nil }

// saveScanBatch writes a batch's results and the new cursor.
func saveScanBatch(context.Context, []int64, int64) error { return nil }

// loadConsumedOffsets reads, for every partition of topic, the offset its
// consumption goes on from: the one stored after the last record applied, or
// the start of a partition never consumed.
func loadConsumedOffsets(context.Context, string) (map[int32]kgo.Offset, error) {
	return map[int32]kgo.Offset{}, nil
}

// applyEvent applies one event to the project's tables.
func applyEvent(context.Context, []byte) error { return nil }

// saveConsumedOffset stores the offset the consumption of a partition goes on
// from.
func saveConsumedOffset(context.Context, string, int32, int64) error { return nil }

// relayOnce relays what is pending.
func relayOnce(context.Context) error { return nil }

// handleUpstreamMessage applies one message of the upstream session.
func handleUpstreamMessage(context.Context, []byte) error { return nil }
