package authz

import (
	"os"
	"regexp"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/middleware"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/router"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

// Register register modules: Permission, Role, UserRole.
//
// Modules:
//   - Permission
//   - Role
//   - UserRole
//   - CasbinRule
//   - Menu
//
// Routes:
//   - GET    /api/authz/permissions
//   - GET    /api/authz/permissions/:id
//   - POST   /api/authz/roles
//   - DELETE /api/authz/roles/:id
//   - PUT    /api/authz/roles/:id
//   - PATCH  /api/authz/roles/:id
//   - GET    /api/authz/roles
//   - GET    /api/authz/roles/:id
//   - POST   /api/authz/user-roles
//   - DELETE /api/authz/user-roles/:id
//   - PUT    /api/authz/user-roles/:id
//   - PATCH  /api/authz/user-roles/:id
//   - GET    /api/authz/user-roles
//   - GET    /api/authz/user-roles/:id
//   - POST   /api/menus
//   - DELETE /api/menus/:id
//   - PUT    /api/menus/:id
//   - PATCH  /api/menus/:id
//   - GET    /api/menus
//   - GET    /api/menus/:id
//   - GET    /api/apis
//   - POST   /api/buttons
//   - DELETE /api/buttons/:id
//   - PUT    /api/buttons/:id
//   - PATCH  /api/buttons/:id
//   - GET    /api/buttons
//   - GET    /api/buttons/:id
//
// Middleware:
//   - Authz
//
// Panic if creates table records failed.
func Register() {
	// Enable RBAC
	os.Setenv(config.AUTH_RBAC_ENABLE, "true")

	// creates table "casbin_rule".
	model.Register[*CasbinRule]()

	// create table "menus" and creates three records.
	model.Register[*Menu](
		&Menu{Base: model.Base{ID: model.RootID}, ParentID: model.RootID},
		&Menu{Base: model.Base{ID: model.NoneID}, ParentID: model.RootID},
		&Menu{Base: model.Base{ID: model.UnknownID}, ParentID: model.RootID},
	)

	// Register auth middleware before protected routes so auth handlers are attached deterministically.
	middleware.RegisterAuth(middleware.Authz())

	module.Use[
		*Permission,
		*Permission,
		*Permission](
		&PermissionModule{},
		consts.PHASE_LIST,
		consts.PHASE_GET,
	)

	module.Use[
		*Role,
		*Role,
		*Role](
		&RoleModule{},
		consts.PHASE_CREATE,
		consts.PHASE_DELETE,
		consts.PHASE_UPDATE,
		consts.PHASE_PATCH,
		consts.PHASE_LIST,
		consts.PHASE_GET,
	)

	module.Use[
		*UserRole,
		*UserRole,
		*UserRole](
		&UserRoleModule{},
		consts.PHASE_CREATE,
		consts.PHASE_DELETE,
		consts.PHASE_UPDATE,
		consts.PHASE_PATCH,
		consts.PHASE_LIST,
		consts.PHASE_GET,
	)

	module.Use[
		*Menu,
		*Menu,
		*Menu](
		&MenuModule{},
		consts.PHASE_CREATE,
		consts.PHASE_DELETE,
		consts.PHASE_UPDATE,
		consts.PHASE_PATCH,
		consts.PHASE_LIST,
		consts.PHASE_GET,
	)

	module.Use[
		*API,
		*API,
		APIRsp](
		&APIModule{},
		consts.PHASE_LIST,
	)

	module.Use[
		*Button,
		*Button,
		*Button](
		&ButtonModule{},
		consts.PHASE_CREATE,
		consts.PHASE_DELETE,
		consts.PHASE_UPDATE,
		consts.PHASE_PATCH,
		consts.PHASE_LIST,
		consts.PHASE_GET,
	)

	log := zap.S()
	go func() {
		for !database.Inited() {
			zap.S().Infow("waiting database inited", "module", "authz")
			time.Sleep(500 * time.Millisecond)
		}

		for !router.Started() {
			zap.S().Infow("waiting router started", "module", "authz")
			time.Sleep(500 * time.Millisecond)
		}

		// re-create all permissions
		if err := database.Database[*modelauthz.Permission](nil).Transaction(func(tx types.Database[*modelauthz.Permission]) error {
			// list all permissions.
			permissions := make([]*modelauthz.Permission, 0)
			if err := tx.List(&permissions); err != nil {
				log.Error(err)
				return err
			}

			// delete all permissions
			if err := tx.WithBatchSize(100).WithPurge().Delete(permissions...); err != nil {
				log.Error(err)
				return err
			}

			// create permissions.
			permissions = make([]*modelauthz.Permission, 0)
			for endpoint, methods := range model.Routes {
				for _, method := range methods {
					permissions = append(permissions, &modelauthz.Permission{
						Resource: convertGinPathToCasbinKeyMatch3(endpoint),
						Action:   method,
					})
				}
			}
			if err := tx.WithBatchSize(100).Create(permissions...); err != nil {
				log.Error(err)
				return err
			}

			return nil
		}); err != nil {
			log.Error(err)
			panic(err)
		}
	}()
}

func convertGinPathToCasbinKeyMatch3(ginPath string) string {
	// Match :param style and replace with {param}
	re := regexp.MustCompile(`:([a-zA-Z0-9_]+)`)
	return re.ReplaceAllString(ginPath, `{$1}`)
}
