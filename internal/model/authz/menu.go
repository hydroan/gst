package modelauthz

import (
	"slices"
	"strings"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/datatypes"
)

func init() {
	// create table "menus" and creates records.
	model.Register[*Menu](
		&Menu{Base: model.Base{ID: model.RootID}, ParentID: model.RootID},
	)
}

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

var MenuRoot = &Menu{ParentID: model.RootID, Base: model.Base{ID: RootID}}

type MenuPlatform string

const (
	MenuPlatformWeb     MenuPlatform = "web"
	MenuPlatformMobile  MenuPlatform = "mobile"
	MenuPlatformDesktop MenuPlatform = "desktop"
)

type Menu struct {
	// Frontend route path. The empty value means default route in React Router 6.x.
	Path    string `json:"path" schema:"path"`
	Default string `json:"default,omitempty" schema:"default"` // Default child route when the menu has children.

	// Backend routes used by this menu.
	Routes datatypes.JSONSlice[Route] `json:"routes,omitempty" schema:"routes"`

	// Display metadata.
	Label string `json:"label,omitempty" schema:"label"`
	Icon  string `json:"icon,omitempty" schema:"icon"`

	// Visibility metadata. Runtime filtering behavior is handled by service logic.
	Visible       *bool                             `json:"visible,omitempty" schema:"visible" gorm:"default:1"`
	Enabled       *bool                             `json:"enabled,omitempty" schema:"enabled" gorm:"default:1"`
	Platforms     datatypes.JSONSlice[MenuPlatform] `json:"platforms,omitempty" schema:"platforms"` // Empty means all platforms.
	DomainPattern string                            `json:"domain_pattern,omitempty" schema:"domain_pattern" gorm:"default:.*"`

	ParentID string  `json:"parent_id,omitempty" gorm:"size:191" schema:"parent_id"`
	Children []*Menu `json:"children,omitempty" gorm:"foreignKey:ParentID"`             // 子路由
	Parent   *Menu   `json:"parent,omitempty" gorm:"foreignKey:ParentID;references:ID"` // 父路由

	model.Base
}

func (Menu) Design() {
	dsl.Migrate(true)
	dsl.Route("menus", func() {
		dsl.Create(func() {})
		dsl.Delete(func() {})
		dsl.Update(func() {})
		dsl.Patch(func() {})
		dsl.List(func() {
			dsl.Service(true)
			dsl.Flatten()
			dsl.Filename("menu.go")
		})
		dsl.Get(func() {})
	})
}

func (m *Menu) Purge() bool                                      { return true }
func (m *Menu) CreateBefore(ctx *types.ModelContext) (err error) { return m.validate() }
func (m *Menu) UpdateBefore(ctx *types.ModelContext) error       { return m.validate() }

// UpdateAfter refreshes permissions for roles that contain the current menu.
func (m *Menu) UpdateAfter(ctx *types.ModelContext) error {
	roles := make([]*Role, 0)
	if err := database.Database[*Role](ctx).List(&roles); err != nil {
		return err
	}
	for _, r := range roles {
		// If the role contains the current menu, then update role's permissions
		if slices.Contains(r.MenuIDs, m.ID) {
			if err := r.UpdatePermission(ctx); err != nil {
				return err
			}
			zap.L().Info("successfully update role's permissions", zap.Object("role", r))
		}
	}

	return nil
}

// DeleteBefore will delete the role's permissions
func (m *Menu) DeleteBefore(ctx *types.ModelContext) error {
	roles := make([]*Role, 0)
	if err := database.Database[*Role](ctx).List(&roles); err != nil {
		return err
	}
	for _, r := range roles {
		// If the role contains the current menu, then update role's permissions
		if slices.Contains(r.MenuIDs, m.ID) {
			// update the role's MenuIDs to remove the current menu
			menuIDs := make([]string, 0)
			for _, mid := range r.MenuIDs {
				if mid != m.ID {
					menuIDs = append(menuIDs, mid)
				}
			}
			r.MenuIDs = menuIDs
			// update the role's MenuPartialIDs to remove the current menu
			menuPartialIDs := make([]string, 0)
			for _, mid := range r.MenuPartialIDs {
				if mid != m.ID {
					menuPartialIDs = append(menuPartialIDs, mid)
				}
			}
			r.MenuPartialIDs = menuPartialIDs

			if err := database.Database[*Role](ctx).Update(r); err != nil {
				return err
			}
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
	if len(m.DomainPattern) == 0 {
		m.DomainPattern = ".*"
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
