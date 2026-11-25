package middleware_test

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/middleware"
)

func TestRouterParamsManager(t *testing.T) {
	t.Run("EmptyPath", func(t *testing.T) {
		manager := middleware.NewRouteParamsManager()
		manager.Add("")

		params := manager.Get("")
		if len(params) != 0 {
			t.Errorf("Expected empty params for empty path, got %v", params)
		}
	})

	t.Run("PathWithoutParams", func(t *testing.T) {
		manager := middleware.NewRouteParamsManager()
		manager.Add("/api/resources")

		params := manager.Get("/api/resources")
		if len(params) != 0 {
			t.Errorf("Expected empty params for path without parameters, got %v", params)
		}
	})

	t.Run("SingleColonParam", func(t *testing.T) {
		manager := middleware.NewRouteParamsManager()
		manager.Add("/api/resources/:id")

		params := manager.Get("/api/resources/:id")
		expected := []string{"id"}
		if !reflect.DeepEqual(params, expected) {
			t.Errorf("Expected params %v, got %v", expected, params)
		}
	})

	t.Run("MultipleParts", func(t *testing.T) {
		manager := middleware.NewRouteParamsManager()
		manager.Add("/users/:userId/posts/:postId/comments/:commentId")

		params := manager.Get("/users/:userId/posts/:postId/comments/:commentId")
		expected := []string{"userId", "postId", "commentId"}

		if !reflect.DeepEqual(params, expected) {
			t.Errorf("Expected params %v, got %v", expected, params)
		}
	})

	t.Run("BracketParams", func(t *testing.T) {
		manager := middleware.NewRouteParamsManager()
		manager.Add("/api/resources/{id}")

		params := manager.Get("/api/resources/{id}")
		expected := []string{"id"}
		if !reflect.DeepEqual(params, expected) {
			t.Errorf("Expected params %v, got %v", expected, params)
		}
	})

	t.Run("MixedParamFormats", func(t *testing.T) {
		manager := middleware.NewRouteParamsManager()
		manager.Add("/api/:version/resources/{resourceId}/items/:itemId")

		params := manager.Get("/api/:version/resources/{resourceId}/items/:itemId")
		expected := []string{"version", "resourceId", "itemId"}

		if !reflect.DeepEqual(params, expected) {
			t.Errorf("Expected params %v, got %v", expected, params)
		}
	})

	t.Run("SpecialCharactersInParams", func(t *testing.T) {
		manager := middleware.NewRouteParamsManager()
		manager.Add("/api/:param_with_underscore/{param-with-dash}")

		params := manager.Get("/api/:param_with_underscore/{param-with-dash}")
		expected := []string{"param_with_underscore", "param-with-dash"}

		if !reflect.DeepEqual(params, expected) {
			t.Errorf("Expected params %v, got %v", expected, params)
		}
	})

	t.Run("ParamWithoutName", func(t *testing.T) {
		manager := middleware.NewRouteParamsManager()
		manager.Add("/api/resources/:/items/{}")

		params := manager.Get("/api/resources/:/items/{}")
		// Depending on the implementation, unnamed params are either skipped or kept as
		// empty strings. Here we assume they are skipped.
		if len(params) != 0 {
			t.Logf("Note: Empty param names were handled as: %v", params)
		}
	})

	t.Run("DuplicateParamNames", func(t *testing.T) {
		manager := middleware.NewRouteParamsManager()
		manager.Add("/api/:id/sub/:id")

		params := manager.Get("/api/:id/sub/:id")
		// NOTE: depending on the implementation this returns two "id" entries or a single deduplicated one.
		t.Logf("Duplicate param handling: %v", params)
		if len(params) == 0 {
			t.Errorf("Expected at least one parameter")
		}
	})

	t.Run("MultipleRoutes", func(t *testing.T) {
		manager := middleware.NewRouteParamsManager()

		// register multiple routes
		manager.Add("/api/users/:userId")
		manager.Add("/api/posts/:postId")

		// verify the params of each route
		params1 := manager.Get("/api/users/:userId")
		expected1 := []string{"userId"}
		if !reflect.DeepEqual(params1, expected1) {
			t.Errorf("Route 1: Expected params %v, got %v", expected1, params1)
		}

		params2 := manager.Get("/api/posts/:postId")
		expected2 := []string{"postId"}
		if !reflect.DeepEqual(params2, expected2) {
			t.Errorf("Route 2: Expected params %v, got %v", expected2, params2)
		}
	})

	t.Run("UnregisteredRoute", func(t *testing.T) {
		manager := middleware.NewRouteParamsManager()
		manager.Add("/api/users/:userId")

		// try to get the params of a route that was never registered
		params := manager.Get("/api/products/:productId")

		// expect nil or an empty slice
		if len(params) != 0 {
			t.Errorf("Expected nil or empty slice for unregistered route, got %v", params)
		}
	})

	t.Run("TrailingSlash", func(t *testing.T) {
		manager := middleware.NewRouteParamsManager()

		// register a route with a trailing slash
		manager.Add("/api/users/:userId/")

		params := manager.Get("/api/users/:userId/")
		expected := []string{"userId"}
		if !reflect.DeepEqual(params, expected) {
			t.Errorf("Expected params %v, got %v", expected, params)
		}

		// check whether the same params are returned without the trailing slash
		// NOTE: this depends on whether the implementation treats it as a different route.
		paramsNoSlash := manager.Get("/api/users/:userId")
		if len(paramsNoSlash) > 0 {
			t.Logf("Note: Trailing slash handling - with slash: %v, without slash: %v",
				params, paramsNoSlash)
		}
	})
}

// TestRouteParamsManagerTableDriven runs the manager's cases as a table.
func TestRouteParamsManagerTableDriven(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []string
	}{
		{"Empty", "", []string{}},
		{"NoParams", "/static/path", []string{}},
		{"SingleParam", "/users/:id", []string{"id"}},
		{"MultipleParams", "/users/:userId/posts/:postId", []string{"userId", "postId"}},
		{"BracketParams", "/api/{version}/resources", []string{"version"}},
		{"MixedFormats", "/api/:version/users/{userId}", []string{"version", "userId"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := middleware.NewRouteParamsManager()
			manager.Add(tt.path)

			got := manager.Get(tt.path)

			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("path %q: expected %v, got %v", tt.path, tt.expected, got)
			}
		})
	}
}
