package main

import (
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/types/consts"
)

func TestRouterTargetForExactAction(t *testing.T) {
	design := &dsl.Design{Param: ":id"}
	action := &dsl.Action{Phase: consts.PHASE_DELETE}
	action.Exact = true

	route, paramName := routerTargetForAction("iam/admin/users/:id/sessions", design, action)

	if route != "iam/admin/users/:id/sessions" {
		t.Fatalf("route = %q, want iam/admin/users/:id/sessions", route)
	}
	if paramName != "id" {
		t.Fatalf("paramName = %q, want id", paramName)
	}
}
