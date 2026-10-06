package controller

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/types"
)

// batch is the body of a batch request: the items to create, update or
// patch, or the ids to delete.
//
// TODO: decide whether a batch update or patch may skip the items whose
// record does not exist and apply the rest, instead of failing whole. A
// switch for it would be a member of this body beside items, and the answer
// would have to list the items it skipped.
//
// TODO: decide whether a batch delete may accept an empty id instead of
// refusing the request, switched on by a member of this body beside ids.
type batch[M types.Model] struct {
	// IDs is the id list that should be batch delete.
	IDs []string `json:"ids,omitempty"`
	// Items is the resource list that should be batch create/update/partial
	// update. Each item is validated against its binding tags the way the
	// body of a single-resource request is: the validator only descends into
	// a slice told to dive. A batch patch validates each item on the fields
	// it names instead, see validatePatchFields.
	Items []M `json:"items,omitempty" binding:"dive"`
}

// itemIDs returns the ids the items of req carry, in order.
func (req *batch[M]) itemIDs() []string {
	ids := make([]string, 0, len(req.Items))
	for _, m := range req.Items {
		ids = append(ids, m.GetID())
	}
	return ids
}

// recordsByID reads the live records of M the ids name in one statement
// through db, the chain prepared the way the caller reads (the primary, the
// expands), and returns them by id; an id naming no live record is absent.
// No ids read nothing.
func (a *action[M, REQ, RSP]) recordsByID(db types.Database[M], ids []string) (map[string]M, error) {
	byID := make(map[string]M, len(ids))
	if len(ids) == 0 {
		return byID, nil
	}
	var records []M
	if err := db.WithQuery(a.newModel(), types.QueryOptions{Filters: []types.Filter{a.idColumn.In(ids...)}}).List(&records); err != nil {
		return nil, err
	}
	for _, record := range records {
		byID[record.GetID()] = record
	}
	return byID, nil
}

// repeatedID returns the error of a batch update or patch whose items name
// one record twice, items[1] names the record "r1", which items[0] already
// names, and nil when every item names a record of its own. The batch
// writes each record once, all of them or none, so a later item naming a
// record an earlier one names would write over it; the batch is refused
// before any record is read. An item naming no record is left to the
// flow, which refuses it.
func (req *batch[M]) repeatedID() error {
	named := make(map[string]int, len(req.Items))
	for i, m := range req.Items {
		id := m.GetID()
		if id == "" {
			continue
		}
		if first, ok := named[id]; ok {
			return errors.Newf("items[%d] names the record %q, which items[%d] already names", i, id, first)
		}
		named[id] = i
	}
	return nil
}
