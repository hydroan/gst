package serviceauthz

import (
	"sort"

	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/router"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type RoutesService struct {
	service.Base[*modelauthz.Routes, *modelauthz.Routes, modelauthz.RoutesRsp]
}

func (RoutesService) List(ctx *types.ServiceContext, req *modelauthz.Routes) (modelauthz.RoutesRsp, error) {
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
	return modelauthz.RoutesRsp{Items: items}, nil
}
