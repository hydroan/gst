package modelauthz

import (
	"context"
	"slices"
	"strings"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/datatypes"
)

var (
	RootID      = model.RootID
	RootName    = model.RootName
	UnknownID   = model.UnknownID
	UnknownName = model.UnknownName
	NoneID      = model.NoneID
	NoneName    = model.NoneName

	KeyName = model.KeyName
	KeyID   = model.KeyID
)

type Menu struct {
	// ID widens the primary key inherited from model.Base.
	//
	// Menu identifiers are stable keys chosen by the application, not generated
	// UUIDs: Role.MenuIDs and every Casbin policy derived from a menu reference
	// them, so they have to survive re-seeding and are therefore written by hand,
	// typically as a hierarchical page key. The inherited column is sized for
	// UUIDv7 and silently truncates anything longer, and ParentID already stores
	// these very same values at size 191 — a primary key narrower than the column
	// pointing at it is a contradiction within one table.
	//
	// Shadowing model.Base.ID makes the ID accessors below mandatory: they are
	// declared on Base and would otherwise read and write the hidden field while
	// GORM maps this one.
	ID string `json:"id" gorm:"primaryKey;size:191" query:"id" url:"-"`

	// Frontend route path. The empty value means default route in React Router 6.x.
	Path    string `json:"path" query:"path"`
	Default string `json:"default,omitempty" query:"default"` // Default child route when the menu has children.

	// Backend routes used by this menu.
	Routes datatypes.JSONSlice[Route] `json:"routes,omitempty" query:"routes"`

	// Display metadata.
	Label string `json:"label,omitempty" query:"label"`

	// Rendering hints for the client. The backend never filters on them: which
	// menus a subject may see is decided solely by Role.MenuIDs.
	Visible *bool `json:"visible,omitempty" query:"visible" gorm:"default:1"`
	Enabled *bool `json:"enabled,omitempty" query:"enabled" gorm:"default:1"`

	// Self-referencing tree. The associations exist so Expands can preload Parent
	// and Children; constraint:- keeps AutoMigrate from emitting a physical foreign
	// key, because preloading resolves the relation in Go and never relies on one.
	// Without that constraint the database no longer rejects deleting a menu that
	// still has children, so callers own the referential integrity of the tree.
	ParentID string  `json:"parent_id,omitempty" gorm:"size:191" query:"parent_id"`
	Children []*Menu `json:"children,omitempty" gorm:"foreignKey:ParentID;constraint:-"`             // Child menus.
	Parent   *Menu   `json:"parent,omitempty" gorm:"foreignKey:ParentID;references:ID;constraint:-"` // Parent menu.

	model.Query
	model.Base
}

func (Menu) Design() {
	dsl.Migrate()
	dsl.Route("authz/menus", func() {
		dsl.Create(func() {})
		dsl.Delete(func() {})
		dsl.Update(func() {})
		dsl.Patch(func() {})
		dsl.List(func() {
			dsl.Service()
			dsl.Flatten()
			dsl.Filename("menu.go")
		})
		dsl.Get(func() {})
	})
}

// GetID reads the shadowing ID field. It replaces model.Base.GetID, which reads
// the shadowed Base field that GORM never maps for this model.
func (m *Menu) GetID() string { return m.ID }

// SetID keeps an identifier the caller already assigned and generates one
// otherwise, matching model.Base.SetID semantics against the shadowing field.
func (m *Menu) SetID(id ...string) {
	if len(m.ID) != 0 {
		return
	}
	if len(id) == 0 || len(id[0]) == 0 {
		m.ID = util.UUID()
		return
	}
	m.ID = id[0]
}

// ClearID resets the shadowing ID field. It replaces model.Base.ClearID, which
// clears the shadowed Base field.
func (m *Menu) ClearID() { m.ID = "" }

func (m *Menu) Purge() bool                                  { return true }
func (m *Menu) CreateBefore(ctx context.Context) (err error) { return m.validate() }
func (m *Menu) UpdateBefore(ctx context.Context) error       { return m.validate() }

// UpdateAfter refreshes permissions for roles that contain the current menu.
func (m *Menu) UpdateAfter(ctx context.Context) error {
	// A menu belongs to no tenant, so what it implies has to be recomputed for
	// the roles of every tenant. Left scoped, this would refresh one tenant's
	// roles and leave the rest holding permissions the menu no longer grants.
	ctx = tenant.Across(ctx)

	roles := make([]*Role, 0)
	if err := database.Database[*Role](ctx).List(&roles); err != nil {
		return err
	}
	for _, r := range roles {
		if slices.Contains(r.MenuIDs, m.ID) {
			if err := r.syncPermissions(ctx); err != nil {
				return err
			}
			zap.L().Info("successfully update role's permissions", zap.Object("role", r))
		}
	}

	return nil
}

// DeleteBefore removes the menu from roles before the menu row is deleted.
func (m *Menu) DeleteBefore(ctx context.Context) error {
	// Same reach as UpdateAfter: the menu is global, so removing it has to be
	// removed from every tenant's roles.
	ctx = tenant.Across(ctx)

	roles := make([]*Role, 0)
	if err := database.Database[*Role](ctx).List(&roles); err != nil {
		return err
	}
	for _, r := range roles {
		if !slices.Contains(r.MenuIDs, m.ID) {
			continue
		}

		r.MenuIDs = removeMenuID(r.MenuIDs, m.ID)

		if err := database.Database[*Role](ctx).Update(r); err != nil {
			return err
		}
	}

	return nil
}

func (m *Menu) Expands() []string { return []string{"Children", "Parent"} }
func (m *Menu) Excludes() map[string][]any {
	return map[string][]any{KeyID: {RootID, UnknownID, NoneID}}
}

func (m *Menu) validate() error {
	if len(m.ParentID) == 0 {
		m.ParentID = RootID
	}
	if m.Visible == nil {
		m.Visible = new(true)
	}
	if m.Enabled == nil {
		m.Enabled = new(true)
	}
	if len(m.Path) > 0 {
		m.Path = strings.TrimSuffix(strings.TrimSpace(m.Path), "/")
	}
	return nil
}

func (m *Menu) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if m == nil {
		return nil
	}
	enc.AddString("routes", strings.Join(routePaths(m.Routes), ","))
	enc.AddString("path", m.Path)
	enc.AddString("label", m.Label)
	enc.AddInt("children len", len(m.Children))

	return nil
}

func routePaths(routes []Route) []string {
	paths := make([]string, 0, len(routes))
	for _, route := range routes {
		if len(route.Path) != 0 {
			paths = append(paths, route.Path)
		}
	}
	return paths
}

func removeMenuID(ids datatypes.JSONSlice[string], menuID string) datatypes.JSONSlice[string] {
	filtered := make(datatypes.JSONSlice[string], 0, len(ids))
	for _, id := range ids {
		if id != menuID {
			filtered = append(filtered, id)
		}
	}
	return filtered
}
