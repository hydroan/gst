// Package tunnel is communication protocol between with server and user, server and client.
package tunnel

import (
	"sync"

	"github.com/vmihailenco/msgpack/v5"
)

type Cmd uint32

var (
	Ping = NewCmd("ping", 1)
	Pong = NewCmd("pong", 2)
)

var cmdMap sync.Map

// NewCmd build a Cmd.
// custom always greater than 1000
func NewCmd(name string, value uint32) Cmd {
	cmdMap.Store(value, name)
	return Cmd(value)
}

func (c Cmd) String() string {
	if v, loaded := cmdMap.Load(uint32(c)); loaded {
		return v.(string) //nolint:errcheck
	}
	return "Unknown"
}

type Event struct {
	ID    string `json:"id,omitempty" msgpack:"id,omitempty"`   // event id
	Cmd   Cmd    `json:"cmd,omitempty" msgpack:"cmd,omitempty"` // event cmd
	Error string `json:"error,omitempty" msgpack:"error,omitempty"`

	Payload any `json:"payload,omitempty" msgpack:"payload,omitempty"`
}

// DecodePayload decodes the payload into the provided value v of type T.
// It handles three cases:
// 1. If the payload is nil, v remains unchanged
// 2. If the payload is already type T, assigns directly to v
// 3. Otherwise, uses msgpack marshal/unmarshal to decode into v
func DecodePayload[T any](payload any, v *T) error {
	// Check nil payload
	if payload == nil {
		return nil
	}

	// Try direct type assertion
	if val, ok := payload.(T); ok {
		*v = val
		return nil
	}

	// Use msgpack marshal/unmarshal for conversion
	b, err := msgpack.Marshal(payload)
	if err != nil {
		return err
	}

	return msgpack.Unmarshal(b, v)
}
