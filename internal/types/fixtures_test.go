package types_test

import "github.com/hydroan/gst/internal/modelregistry"

// The models the tests of this package build column references against. They
// live here rather than in one feature's test file because the tests of
// several features build references against the same models.

// sampleStatus is a named string type, standing in for a model enum.
type sampleStatus string

const (
	sampleStatusActive  sampleStatus = "active"
	sampleStatusRemoved sampleStatus = "removed"
)

// sampleTable stands in for the model a column reference is generated for;
// only its table name takes part.
type sampleTable struct{}

func (sampleTable) TableName() string { return "samples" }

// sampleRecord is a model a join can name: Join needs the full model
// contract, which sampleTable, a bare table namer, does not carry.
type sampleRecord struct {
	Code string `json:"code"`

	modelregistry.Base
}

func (*sampleRecord) TableName() string { return "sample_records" }
