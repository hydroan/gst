package modelschema

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// sampleTreeRecord declares the two associations a model preloads, a has-many
// and a belongs-to, beside its own columns.
type sampleTreeRecord struct {
	ID       string              `json:"id" gorm:"primaryKey"`
	Name     string              `json:"name"`
	ParentID string              `json:"parent_id"`
	Children []*sampleTreeRecord `json:"children" gorm:"foreignKey:ParentID"`
	Parent   *sampleTreeRecord   `json:"parent" gorm:"foreignKey:ParentID;references:ID"`
}

func (sampleTreeRecord) TableName() string { return "sample_tree_records" }

// sampleOwnerRecord holds sampleOwnedRecord rows through a has-many, which
// gorm registers on sampleOwnedRecord's schema as well, under a hidden name,
// once it parses the owner.
type sampleOwnerRecord struct {
	ID      string               `json:"id" gorm:"primaryKey"`
	Entries []*sampleOwnedRecord `json:"entries" gorm:"foreignKey:OwnerID"`
}

func (sampleOwnerRecord) TableName() string { return "sample_owner_records" }

// sampleOwnedRecord is held by a sampleOwnerRecord and declares no
// association of its own.
type sampleOwnedRecord struct {
	ID      string `json:"id" gorm:"primaryKey"`
	OwnerID string `json:"owner_id"`
}

func (sampleOwnedRecord) TableName() string { return "sample_owned_records" }

// TestRecursiveAssociationsListsTheToManyHoldingTheTypeItself pins which
// associations an expanding depth repeats: a has-many or many-to-many
// holding the model itself, the Children of a tree, and not a single-valued
// one holding it, a has-many of another model, or what gorm registers on a
// held model for the one holding it.
func TestRecursiveAssociationsListsTheToManyHoldingTheTypeItself(t *testing.T) {
	type samplePeerRecord struct {
		ID    string              `gorm:"primaryKey"`
		Peers []*samplePeerRecord `gorm:"many2many:sample_peer_links"`
	}

	require.Equal(t, map[string]struct{}{"Children": {}}, RecursiveAssociations(reflect.TypeFor[*sampleTreeRecord]()))
	require.Equal(t, map[string]struct{}{"Peers": {}}, RecursiveAssociations(reflect.TypeFor[samplePeerRecord]()))
	require.Empty(t, RecursiveAssociations(reflect.TypeFor[sampleOwnerRecord]()))
	require.Empty(t, RecursiveAssociations(reflect.TypeFor[sampleOwnedRecord]()))
	require.Nil(t, RecursiveAssociations(reflect.TypeFor[string]()), "a type without a resolvable schema has none")
}

// TestAssociationFieldsListsTheRelationshipFields pins the field paths of a
// model's associations — the relationships it declares itself, not the ones
// gorm registers on it for a model holding it — the per-type caching, and
// the nil-on-failure contract.
func TestAssociationFieldsListsTheRelationshipFields(t *testing.T) {
	paths := AssociationFields(reflect.TypeFor[*sampleTreeRecord]())
	require.Equal(t, [][]int{{3}, {4}}, paths)

	second := AssociationFields(reflect.TypeFor[sampleTreeRecord]())
	require.Equal(t, reflect.ValueOf(paths).Pointer(), reflect.ValueOf(second).Pointer(),
		"pointer and struct type must share one cached list")

	// Resolving the owner first registers its has-many on the owned model's
	// schema; that entry describes the owner's field.
	require.Equal(t, [][]int{{1}}, AssociationFields(reflect.TypeFor[sampleOwnerRecord]()))
	require.Empty(t, AssociationFields(reflect.TypeFor[sampleOwnedRecord]()), "the owner's has-many is no association of the owned model")

	require.Empty(t, AssociationFields(reflect.TypeFor[sampleDocumentRecord]()), "a model without associations has none")
	require.Nil(t, AssociationFields(reflect.TypeFor[string]()),
		"a type without resolvable columns yields nil, not an error")
}

// sampleHolderRecord holds sampleHeldRecord rows through a has-many, so
// parsing it writes into sampleHeldRecord's cached schema.
type sampleHolderRecord struct {
	ID      string              `json:"id" gorm:"primaryKey"`
	Entries []*sampleHeldRecord `json:"entries" gorm:"foreignKey:HolderID"`
}

func (sampleHolderRecord) TableName() string { return "sample_holder_records" }

// sampleHeldRecord is held by a sampleHolderRecord.
type sampleHeldRecord struct {
	ID       string `json:"id" gorm:"primaryKey"`
	HolderID string `json:"holder_id"`
}

func (sampleHeldRecord) TableName() string { return "sample_held_records" }

// TestAssociationFieldsWalksUnderTheSchemaLock pins that the walk over a
// type's relationships takes the lock gorm writes them under: parsing a
// holder registers its has-many on the held type's schema, and here that
// parse runs while another goroutine keeps resolving the held type. The race
// detector, which make test runs under, reports a walk without the lock.
func TestAssociationFieldsWalksUnderTheSchemaLock(t *testing.T) {
	held := reflect.TypeFor[sampleHeldRecord]()
	require.Empty(t, AssociationFields(held))

	stop := make(chan struct{})
	walked := make(chan struct{})
	go func() {
		defer close(walked)
		for {
			select {
			case <-stop:
				return
			default:
			}
			associationFieldsCache.Delete(held)
			AssociationFields(held)
		}
	}()
	require.Equal(t, [][]int{{1}}, AssociationFields(reflect.TypeFor[sampleHolderRecord]()))
	time.Sleep(50 * time.Millisecond)
	close(stop)
	<-walked
}
