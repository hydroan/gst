package modelauthz

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
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
	// UUIDs: Role.MenuIDs and every policy row derived from a menu reference
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

func (m *Menu) Purge() bool { return true }

func (m *Menu) CreateBefore(ctx context.Context) (err error) {
	if err := errIfMenuWriteForbidden(ctx); err != nil {
		return err
	}
	return m.validate()
}

func (m *Menu) UpdateBefore(ctx context.Context) error {
	if err := errIfMenuWriteForbidden(ctx); err != nil {
		return err
	}
	return m.validate()
}

// errIfMenuWriteForbidden refuses a menu write from a subject that is not
// system-level.
//
// A menu is global, and what it carries is authorization: its routes are what
// every tenant's roles derive their backend permissions from, so writing one
// reaches into every tenant. The authorization middleware cannot draw that
// line — a tenant administrator passes it for any object inside their tenant,
// and these are the objects through which one tenant's administrator would
// reshape every other tenant's grants. The boundary therefore sits on the
// write itself, where every path to it converges. Reads stay as they are:
// which menus a subject sees is decided per tenant by role bindings.
//
// A write with no subject behind it is not refused. Authorization rejects
// anonymous requests before any handler runs, so no subject means no request:
// seeding, a job, framework code — the deployment's own hand.
func errIfMenuWriteForbidden(ctx context.Context) error {
	subject := strings.TrimSpace(types.RequestUserID(ctx))
	if subject == "" {
		return nil
	}
	systemRoot, err := rbac.RBAC().HasSystemRole(ctx, subject, consts.AUTHZ_SYSTEM_ROLE_ROOT)
	if err != nil {
		return err
	}
	if !systemRoot {
		return service.NewError(http.StatusForbidden, "only system administrators may modify menus")
	}
	return nil
}

// UpdateAfter refreshes permissions for roles that contain the current menu.
func (m *Menu) UpdateAfter(ctx context.Context) error {
	// A menu belongs to no tenant, so what it implies has to be recomputed for
	// the roles of every tenant. Left scoped, this would refresh one tenant's
	// roles and leave the rest holding permissions the menu no longer grants.
	ctx = tenant.Across(ctx)

	roles, err := rolesToRefresh(ctx)
	if err != nil {
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

// rolesToRefresh reads the roles a menu write has to act on, holding each under
// an exclusive row lock for the rest of the menu's transaction.
//
// The lock is what makes rewriting a role's permissions from here safe. A
// permission set is replaced by deleting the role's policy rows and inserting
// the new ones, and two transactions doing that to one role at the same time
// leave the union of both sets rather than the later one: on PostgreSQL neither
// statement sees rows the other has not committed yet, so neither delete
// removes the other's insert. Storage then holds permissions the deciding
// process does not, and keeps them until something reloads it.
//
// A role's own write path never needed this. It has already written the role
// row by the time it refreshes permissions, so it holds that row for the rest
// of its transaction and a second writer waits. A menu write reaches the same
// roles without touching their rows, which is what leaves it the one path that
// can interleave — and taking the lock here is what puts it on the same footing
// rather than on PostgreSQL's statement visibility.
//
// It has to be a lock rather than a re-read: the roles being replaced may hold
// no policy rows at all, and there is nothing to serialize on in a range that
// is empty.
func rolesToRefresh(ctx context.Context) ([]*Role, error) {
	roles := make([]*Role, 0)
	if err := database.Database[*Role](ctx).WithLock(consts.LockUpdate).List(&roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// DeleteBefore removes the menu from roles before the menu row is deleted.
func (m *Menu) DeleteBefore(ctx context.Context) error {
	if err := errIfMenuWriteForbidden(ctx); err != nil {
		return err
	}

	// Same reach as UpdateAfter: the menu is global, so removing it has to be
	// removed from every tenant's roles.
	ctx = tenant.Across(ctx)

	roles, err := rolesToRefresh(ctx)
	if err != nil {
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
