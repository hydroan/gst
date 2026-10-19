package tunnel

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"uuid"

	"github.com/cockroachdb/errors"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/zap"
)

// AppSide tells NewSession which end of a connection the caller is.
type AppSide string

const (
	Server AppSide = "server"
	Client AppSide = "client"
)

// maxFrameSize bounds the encoded event a frame carries, the size its header
// announces: 4 MiB, grpc-go's default limit on a received message.
const maxFrameSize = 4 << 20

// connMap registers the connection of every session under the key NewSession
// gives it; the values are *connMapVal.
var connMap sync.Map

// connMapVal is a registered connection, shared by every session on it.
type connMapVal struct {
	key  any
	conn net.Conn

	// rmu makes reading a frame's header and its body one step, and wmu
	// writing them; without them, concurrent reads or writes on the
	// connection interleave frames.
	rmu sync.Mutex
	wmu sync.Mutex
}

// List returns the registered connections by their keys, see NewSession.
func List() map[any]any {
	kv := make(map[any]any)
	connMap.Range(func(key, val any) bool {
		kv[key] = val
		return true
	})
	return kv
}

type Session struct {
	tcpconn net.Conn

	locker *connMapVal
}

// Read reads and decodes an Event from the connection.
// The binary protocol format is:
//  1. First 4 bytes: data size in BigEndian uint32
//  2. Remaining bytes: msgpack encoded Event data
//
// Concurrent reads take whole frames in turn.
//
// Returns an error if:
//   - Failed to read the data size
//   - Data size is 0 or over 4 MiB, which no Write produces; the session is
//     closed as well, since the peer is out of step with the protocol and,
//     past an oversize header, the stream holds no header to read next
//   - Failed to read the event data
//   - Failed to unmarshal the data into Event
func (s *Session) Read() (*Event, error) {
	if s.tcpconn == nil {
		return nil, errors.New("connection is nil")
	}
	s.locker.rmu.Lock()
	defer s.locker.rmu.Unlock()

	// 1.read data size
	var size uint32
	if err := binary.Read(s.tcpconn, binary.BigEndian, &size); err != nil {
		return nil, err
	}
	if size == 0 || size > maxFrameSize {
		return nil, errors.CombineErrors(
			errors.Newf("invalid data size: received %d, want 1 to %d", size, maxFrameSize),
			s.Close(),
		)
	}
	// 2.read data
	cmdBuf := make([]byte, size)
	if _, err := io.ReadFull(s.tcpconn, cmdBuf); err != nil {
		return nil, err
	}
	binaryLog().Infoz("Read", zap.Uint32("size", size), zap.Binary("binary", cmdBuf))
	// 3.unmarshal data.
	event := &Event{}
	if err := msgpack.Unmarshal(cmdBuf, event); err != nil {
		return nil, err
	}

	protocolLog().Infow("Session.Read()", "key", fmt.Sprint(s.locker.key), "cmd", event.Cmd)
	return event, nil
}

// Write encodes and writes an Event to the connection.
// The binary protocol format is:
//  1. First 4 bytes: data size in BigEndian uint32
//  2. Remaining bytes: msgpack encoded Event data
//
// If the Event.ID is empty, it will be set to a new UUID.
// The write operation is synchronized using a mutex to ensure atomic writes.
// Returns an error if:
//   - Failed to marshal the event
//   - The encoded event is over 4 MiB, in which case nothing is written
//   - Failed to write the data size
//   - Failed to write the event data
func (s *Session) Write(event *Event) error {
	if s.tcpconn == nil {
		return errors.New("connection is nil")
	}
	if event == nil {
		return nil
	}
	protocolLog().Infow("Session.Write() S", "key", fmt.Sprint(s.locker.key), "cmd", event.Cmd) // S: start
	s.locker.wmu.Lock()
	protocolLog().Infow("Session.Write() L", "key", fmt.Sprint(s.locker.key), "cmd", event.Cmd) // L: lock
	defer s.locker.wmu.Unlock()
	// This ID is critical: never overwrite an ID the caller already set,
	// because events sharing the same ID belong to the same ongoing exchange,
	// while a different ID marks a different one.
	if event.ID == "" {
		event.ID = uuid.NewV7().String()
	}
	// 1.marshal data
	buf, err := msgpack.Marshal(event)
	if err != nil {
		return err
	}
	if len(buf) > maxFrameSize {
		return errors.Newf("invalid data size: encoded %d, want at most %d", len(buf), maxFrameSize)
	}

	// 2.write data size
	if err := binary.Write(s.tcpconn, binary.BigEndian, uint32(len(buf))); err != nil { //nolint:gosec
		return err
	}
	binaryLog().Infoz("Write", zap.Int("size", len(buf)), zap.Binary("binary", buf))
	// 3.write data
	if err := binary.Write(s.tcpconn, binary.BigEndian, buf); err != nil {
		return err
	}
	return nil
}

// Close closes the session's connection, which ends every session on it, and
// takes the connection out of the registration List reads, unless another
// connection has taken its key over since.
func (s *Session) Close() error {
	if s.tcpconn == nil {
		return errors.New("connection is nil")
	}
	connMap.CompareAndDelete(s.locker.key, s.locker)
	return s.tcpconn.Close()
}

// NewSession creates a new tunnel session from a connection.
// Currently only supports TCP connections (*net.TCPConn).
//
// Parameters:
//   - _conn: The connection to create session from, must be *net.TCPConn
//   - appSide: Indicates whether this is server or client side (Server/Client)
//   - cid: Optional client ID, only used on server side. If provided, will be used as the connection key
//
// The connection key is the client ID on the server side when one is given,
// and otherwise the connection's local and remote addresses, such as
// "127.0.0.1:9000->127.0.0.1:52144": the pair names one end of one connection
// on the host, where the remote address alone does not, since every
// connection a client opens to one server has the same remote address, and
// two listeners may see the same remote address too.
//
// Every session on one connection shares the connection's registration, and
// with it the locks that keep reads and writes whole. A key another connection
// holds is taken over: the newer connection replaces the older one, which is
// closed, the way MQTT treats a client ID that connects again; a client often
// reconnects before the server has noticed its older connection is gone.
//
// Returns:
//   - *Session and nil if successful
//   - nil and error if the connection is not a *net.TCPConn or appSide is neither Server nor Client
func NewSession(_conn any, appSide AppSide, cid ...string) (*Session, error) {
	conn, ok := _conn.(*net.TCPConn)
	if !ok {
		return nil, errors.New("only support tcp.Conn")
	}
	var key any = conn.LocalAddr().String() + "->" + conn.RemoteAddr().String()
	switch appSide {
	case Server:
		if len(cid) > 0 && len(cid[0]) > 0 {
			key = cid[0]
		}
	case Client:
	default:
		return nil, errors.Newf("unknown app side %q, want %q or %q", appSide, Server, Client)
	}

	locker := &connMapVal{key: key, conn: conn}
	for {
		val, loaded := connMap.LoadOrStore(key, locker)
		if !loaded {
			break
		}
		registered := val.(*connMapVal) //nolint:errcheck
		if registered.conn == conn {
			locker = registered
			break
		}
		if connMap.CompareAndSwap(key, registered, locker) {
			if err := registered.conn.Close(); err != nil {
				protocolLog().Warnw("NewSession: failed to close the connection taken over", "key", fmt.Sprint(key), "err", err)
			}
			break
		}
	}
	return &Session{tcpconn: conn, locker: locker}, nil
}
