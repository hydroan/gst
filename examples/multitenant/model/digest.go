package model

import (
	"time"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/tenant"
)

// Digest is a tenant's count of its tickets at one moment, written by the
// job in cronjob, which acts in each tenant in turn through tenant.In: a
// row the stamp files under the tenant the job acted in, read back by that
// tenant alone. GET /api/digests and /api/digests/:id.
type Digest struct {
	Open   int       `json:"open" query:"open"`
	Closed int       `json:"closed" query:"closed"`
	At     time.Time `json:"at" query:"at"`

	tenant.Scope
	model.Query
	model.Base
}

func (Digest) TableName() string { return "digests" }

func (Digest) Design() {
	Migrate()
	Endpoint("digests")

	List(func() {})
	Get(func() {})
}
