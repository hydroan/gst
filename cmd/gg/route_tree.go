package main

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/ds/tree/trie"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/spf13/cobra"
)

// routeTreeInfo holds what the URL route tree prints for a route: its HTTP
// methods.
type routeTreeInfo struct {
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
	RunE: func(cmd *cobra.Command, args []string) error {
		var filter string
		if len(args) > 0 {
			filter = args[0]
		}
		return routeTreeRun(cmd.OutOrStdout(), filter)
	},
}

// routeTreeRun prints to w the URL route tree of the generated router file,
// keeping only the routes whose path holds filter when one is given.
func routeTreeRun(w io.Writer, filter string) error {
	routes, err := parseRouteTreeFromFile()
	if err != nil {
		return errors.Wrap(err, "failed to parse routes")
	}

	if len(routes) == 0 {
		fmt.Fprintln(w, "No routes found. Please run 'gg gen' first to generate routes.")
		return nil
	}

	if filter != "" {
		routes = filterRouteTree(routes, filter)
		if len(routes) == 0 {
			fmt.Fprintf(w, "No routes found matching filter: %s\n", filter)
			return nil
		}
	}

	return printRouteTree(w, routes)
}

// filterRouteTree keeps the URL route tree entries whose path holds filter.
func filterRouteTree(routes map[string][]string, filter string) map[string][]string {
	filteredRoutes := make(map[string][]string)
	filter = strings.Trim(filter, "/")

	for path, methods := range routes {
		if strings.Contains(strings.TrimPrefix(path, "/"), filter) {
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
	registered, err := parseModelRoutesFromProject(filepath.Join(ggconst.DirRouter, ggconst.FileRouterGen), ggconst.DirModel)
	if err != nil {
		return nil, err
	}

	routes := make(map[string][]string)
	for _, route := range registered {
		routes[route.Path] = append(routes[route.Path], route.Method)
	}
	return routes, nil
}

// printRouteTree builds the URL route tree structure and prints it to w, the
// methods colored the way gg routes colors them when w is a terminal.
func printRouteTree(w io.Writer, routes map[string][]string) error {
	trie, err := trie.New[string, *routeTreeInfo]()
	if err != nil {
		return errors.Wrap(err, "failed to build the route tree")
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
		trie.Put(filteredSegments, &routeTreeInfo{Methods: methods})
	}

	fmt.Fprintln(w, "Route Tree Structure:")
	fmt.Fprintln(w, "=====================")
	if trie.IsEmpty() {
		fmt.Fprintln(w, "No routes found.")
		return nil
	}

	printRouteTreeNode(w, trie.Root(), "", routeColorEnabled(w))
	return nil
}

func printRouteTreeNode(w io.Writer, node *trie.Node[string, *routeTreeInfo], childPrefix string, colorize bool) {
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
			methodsStr := formatRouteTreeMethods(route.Methods, colorize)
			if isLast {
				fmt.Fprintf(w, "%s└─ %s %s\n", childPrefix, child.key, methodsStr)
			} else {
				fmt.Fprintf(w, "%s├─ %s %s\n", childPrefix, child.key, methodsStr)
			}
			continue
		}

		if isLast {
			fmt.Fprintf(w, "%s└─ %s/\n", childPrefix, child.key)
			newPrefix += "   "
		} else {
			fmt.Fprintf(w, "%s├─ %s/\n", childPrefix, child.key)
			newPrefix += "│  "
		}

		if hasRoute {
			route := child.node.Value()
			methodsStr := formatRouteTreeMethods(route.Methods, colorize)
			if isLast {
				fmt.Fprintf(w, "%s   ● %s\n", childPrefix, methodsStr)
			} else {
				fmt.Fprintf(w, "%s│  ● %s\n", childPrefix, methodsStr)
			}
		}

		printRouteTreeNode(w, child.node, newPrefix, colorize)
	}
}

func formatRouteTreeMethods(methods []string, colorize bool) string {
	formatted := make([]string, 0, len(methods))
	for _, method := range methods {
		formatted = append(formatted, routeText(colorize, routeMethodStyle(method), "%s", method))
	}
	return "[" + strings.Join(formatted, ", ") + "]"
}
