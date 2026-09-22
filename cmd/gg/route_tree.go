package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hydroan/gst/ds/tree/trie"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/spf13/cobra"
)

// routeTreeInfo represents route tree endpoint information.
type routeTreeInfo struct {
	Path    string
	Methods []string
}

var routeTreeCmd = &cobra.Command{
	Use:   "route-tree [filter]",
	Short: "print current URL route tree structure",
	Long: `Print the current URL route tree structure in a hierarchical format.
This command analyzes the registered routes and displays them as a URL-first tree.

Optional filter parameter can be used to show only routes matching the specified pattern.
For example: 'gg route-tree samples/items' will show only routes under samples/items.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var filter string
		if len(args) > 0 {
			filter = args[0]
		}
		routeTreeRun(filter)
	},
}

func routeTreeRun(filter string) {
	routes, err := parseRouteTreeFromFile()
	if err != nil {
		fmt.Printf("Error parsing routes: %v\n", err)
		os.Exit(1)
	}

	if len(routes) == 0 {
		fmt.Println("No routes found. Please run 'gg gen' first to generate routes.")
		return
	}

	if filter != "" {
		routes = filterRouteTree(routes, filter)
		if len(routes) == 0 {
			fmt.Printf("No routes found matching filter: %s\n", filter)
			return
		}
	}

	printRouteTree(routes)
}

// filterRouteTree filters URL route tree entries by path.
func filterRouteTree(routes map[string][]string, filter string) map[string][]string {
	filteredRoutes := make(map[string][]string)
	filter = strings.Trim(filter, "/")

	for path, methods := range routes {
		normalizedPath := strings.TrimPrefix(path, "/")
		if strings.HasPrefix(normalizedPath, filter) || strings.Contains(normalizedPath, filter) {
			filteredRoutes[path] = methods
		}
	}

	return filteredRoutes
}

// parseRouteTreeFromFile reads the routes of the generated
// router/router.gen.go the way gg routes does, see
// parseModelRoutesFromProject, and keys their HTTP methods by path for the
// URL route tree.
func parseRouteTreeFromFile() (map[string][]string, error) {
	registered, err := parseModelRoutesFromProject(filepath.Join(routerDir, ggconst.FileRouterGen), modelDir)
	if err != nil {
		return nil, err
	}

	routes := make(map[string][]string)
	for _, route := range registered {
		routes[route.Path] = append(routes[route.Path], route.Method)
	}
	return routes, nil
}

// printRouteTree builds and prints the URL route tree structure.
func printRouteTree(routes map[string][]string) {
	trie, err := trie.New[string, *routeTreeInfo]()
	if err != nil {
		fmt.Printf("Error creating trie: %v\n", err)
		return
	}

	for path, methods := range routes {
		segments := strings.Split(strings.Trim(path, "/"), "/")
		filteredSegments := make([]string, 0, len(segments))
		for _, segment := range segments {
			if segment != "" {
				filteredSegments = append(filteredSegments, segment)
			}
		}

		sort.Strings(methods)
		trie.Put(filteredSegments, &routeTreeInfo{
			Path:    path,
			Methods: methods,
		})
	}

	fmt.Println("Route Tree Structure:")
	fmt.Println("=====================")
	if trie.IsEmpty() {
		fmt.Println("No routes found.")
		return
	}

	printRouteTreeNode(trie.Root(), "", "")
}

func printRouteTreeNode(node *trie.Node[string, *routeTreeInfo], _ string, childPrefix string) {
	if node == nil {
		return
	}

	children := node.Children()
	type childPair struct {
		key  string
		node *trie.Node[string, *routeTreeInfo]
	}
	childList := make([]childPair, 0, len(children))
	for k, v := range children {
		childList = append(childList, childPair{k, v})
	}
	sort.Slice(childList, func(i, j int) bool {
		return childList[i].key < childList[j].key
	})

	for i, child := range childList {
		isLast := i == len(childList)-1
		newPrefix := childPrefix
		hasRoute := child.node.Value() != nil
		hasChildren := len(child.node.Children()) > 0

		if hasRoute && !hasChildren {
			route := child.node.Value()
			methodsStr := formatRouteTreeMethods(route.Methods)
			if isLast {
				fmt.Printf("%s└─ %s %s\n", childPrefix, child.key, methodsStr)
			} else {
				fmt.Printf("%s├─ %s %s\n", childPrefix, child.key, methodsStr)
			}
			continue
		}

		if isLast {
			fmt.Printf("%s└─ %s/\n", childPrefix, child.key)
			newPrefix += "   "
		} else {
			fmt.Printf("%s├─ %s/\n", childPrefix, child.key)
			newPrefix += "│  "
		}

		if hasRoute {
			route := child.node.Value()
			methodsStr := formatRouteTreeMethods(route.Methods)
			if isLast {
				fmt.Printf("%s   ● %s\n", childPrefix, methodsStr)
			} else {
				fmt.Printf("%s│  ● %s\n", childPrefix, methodsStr)
			}
		}

		printRouteTreeNode(child.node, "", newPrefix)
	}
}

func formatRouteTreeMethods(methods []string) string {
	var formatted []string
	for _, method := range methods {
		switch method {
		case "GET":
			formatted = append(formatted, "\033[32mGET\033[0m")
		case "POST":
			formatted = append(formatted, "\033[34mPOST\033[0m")
		case "PUT":
			formatted = append(formatted, "\033[33mPUT\033[0m")
		case "PATCH":
			formatted = append(formatted, "\033[35mPATCH\033[0m")
		case "DELETE":
			formatted = append(formatted, "\033[31mDELETE\033[0m")
		default:
			formatted = append(formatted, method)
		}
	}
	return "[" + strings.Join(formatted, ", ") + "]"
}
