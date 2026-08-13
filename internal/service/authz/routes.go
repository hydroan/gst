package serviceauthz

import (
	"sort"

	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/router"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// RoutesService lists every registered backend route for menu binding.
//
// The request type is *model.Empty to match the DSL: a List action that only
// declares a Result always parses with an empty payload, and the add path must
// register the same request type the copy path generates.
type RoutesService struct {
	service.Base[*modelauthz.Routes, *model.Empty, *modelauthz.RoutesRsp]
}

func (RoutesService) List(ctx *types.ServiceContext, req *model.Empty) (*modelauthz.RoutesRsp, error) {
	routes := router.Routes()
	items := make([]modelauthz.Route, 0, len(routes))
	for path, methods := range routes {
		items = append(items, modelauthz.Route{
			Path:    path,
			Methods: methods,
		})
	}
	sort.Slice(items, func(i int, j int) bool {
		return items[i].Path < items[j].Path
	})
	return &modelauthz.RoutesRsp{Items: items}, nil
}
