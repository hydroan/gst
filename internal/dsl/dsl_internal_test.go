package dsl

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/consts"
)

func TestDesignRangeOrderDefaultRoute(t *testing.T) {
	design := parseDesignFromSource(t, defaultRouteOrderSource, "OrderSample")

	var got []consts.Phase
	design.Range(func(route string, act *Action) {
		got = append(got, act.Phase)
	})

	want := []consts.Phase{
		consts.List,
		consts.Import,
		consts.Export,
		consts.Get,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected action order: got %v want %v", got, want)
	}
}

func TestDesignRangeOrderCustomRoute(t *testing.T) {
	design := parseDesignFromSource(t, routeOrderSource, "RouteSample")

	var got []consts.Phase
	design.Range(func(route string, act *Action) {
		if route == "sample/records" {
			got = append(got, act.Phase)
		}
	})

	want := []consts.Phase{
		consts.List,
		consts.Import,
		consts.Export,
		consts.Get,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected route action order: got %v want %v", got, want)
	}
}

const defaultRouteOrderSource = `
package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type OrderSample struct {
	model.Base
}

func (OrderSample) Design() {
	Endpoint("sample/records")
	Get(func() {
	})
	Export(func() {
	})
	Import(func() {
	})
	List(func() {
	})
}
`

const routeOrderSource = `
package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type RouteSample struct {
	model.Base
}

func (RouteSample) Design() {
	Route("/sample/records", func() {
		Get(func() {
		})
		Export(func() {
		})
		Import(func() {
		})
		List(func() {
		})
	})
}
`
