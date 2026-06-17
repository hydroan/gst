package modelauthz

import (
	"slices"
	"strings"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
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

var MenuRoot = &Menu{ParentID: model.RootID, Base: model.Base{ID: RootID}}

type MenuPlatform string

const (
	MenuPlatformAll     = "all"
	MenuPlatformWeb     = "web"
	MenuPlatformMobile  = "mobile"
	MenuPlatformDesktop = "desktop"
)

type Menu struct {
	API   datatypes.JSONSlice[string] `json:"api,omitempty" schema:"api"`     // 后端路由, 如果为空则使用 "/api" + Path
	Path  string                      `json:"path,omitempty" schema:"path"`   // path should not add `omitempty` tag, empty value means default router in react route6.x.
	Label string                      `json:"label,omitempty" schema:"label"` // 页面组件左侧的菜单名
	Icon  string                      `json:"icon,omitempty" schema:"icon"`   // 页面组件左侧的菜单图标

	Visiable *bool  `json:"visiable,omitempty" schema:"visiable" gorm:"default:1"`                                                   // 前端页面路由是否可见
	Default  string `json:"default,omitempty" schema:"default"`                                                                      // 子路由中的默认路由, 如果有 Children, Default 才可能存在
	Status   *uint  `json:"status,omitempty" gorm:"type:smallint;default:1;comment:status(0: disabled, 1: enabled)" schema:"status"` // 该路由是否启用

	ParentID string  `json:"parent_id,omitempty" gorm:"size:191" schema:"parent_id"`
	Children []*Menu `json:"children,omitempty" gorm:"foreignKey:ParentID"`             // 子路由
	Parent   *Menu   `json:"parent,omitempty" gorm:"foreignKey:ParentID;references:ID"` // 父路由

	// the empty value of `Platform` means all.
	Platform MenuPlatform `json:"platform,omitempty" schema:"platform"`

	DomainPattern string `json:"domain_pattern,omitempty" schema:"domain_pattern" gorm:"default:.*"`

	model.Base
}

func (m *Menu) Purge() bool                                      { return true }
func (m *Menu) CreateBefore(ctx *types.ModelContext) (err error) { return m.validate() }
func (m *Menu) UpdateBefore(ctx *types.ModelContext) error       { return m.validate() }

// UpdateAfter will query all roles and check whether the role contains the current menu.
// If role contains current menu and the menu's API changed,
// then call "role.UpdatePermission" to updates the role's permissions.
func (m *Menu) UpdateAfter(ctx *types.ModelContext) error {
	// // // if update not contains "API", skip update role's permissions
	// // if len(m.API) == 0 {
	// // 	return nil
	// // }
	//
	// // update "API" but we should check whether the original menu's API and
	// // current updates menu's API are the same.
	// //
	// // query the original menu from database.
	// orig := new(Menu)
	// if err := database.Database[*Menu](ctx.DatabaseContext()).Get(orig, m.ID); err != nil {
	// 	return err
	// }
	//
	// // // if the original menu's API and current updates menu's API are the same,
	// // // skip update role's permissions
	// // if reflect.DeepEqual(orig.API, m.API) {
	// // 	zap.S().Info("menu's api not changed, skip update role's permissions")
	// // 	return nil
	// // }

	roles := make([]*Role, 0)
	if err := database.Database[*Role](ctx.DatabaseContext()).List(&roles); err != nil {
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
	if err := database.Database[*Role](ctx.DatabaseContext()).List(&roles); err != nil {
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

			if err := database.Database[*Role](ctx.DatabaseContext()).Update(r); err != nil {
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
	if m.Visiable == nil {
		m.Visiable = new(true)
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
	enc.AddString("api", strings.Join(m.API, ","))
	enc.AddString("path", m.Path)
	enc.AddString("label", m.Label)
	enc.AddInt("children len", len(m.Children))

	return nil
}
