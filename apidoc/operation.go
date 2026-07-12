package apidoc

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/hydroan/gst/types/consts"
)

// Operation describes one API operation when the OpenAPI document is built.
// It is the input of the summary and description generators.
type Operation struct {
	// Method is the HTTP request method, eg. "POST".
	Method string
	// Path is the route path, eg. "/api/groups/{id}/disable".
	Path string
	// Verb is the framework action verb, eg. consts.Create.
	Verb consts.HTTPVerb
	// CustomTypes reports whether the operation declares its own request and
	// response types instead of reusing the model (custom action routes do,
	// default CRUD routes do not).
	CustomTypes bool
	// ModelName is the Go type name of the model backing the route.
	ModelName string
	// ModelComment is the API-facing doc comment of the model, "" when absent.
	ModelComment string
}

// OperationDoc holds an explicit summary and description for one operation.
// Registered entries take precedence over the generators, mirroring how an
// explicitly filled huma.Operation beats huma's generated defaults.
type OperationDoc struct {
	// Summary overrides the operation summary when non-empty.
	Summary string
	// Description overrides the operation description when non-empty.
	Description string
}

// GenerateSummary and GenerateDescription build the operation summary and
// description when no explicit OperationDoc is registered. They default to
// DefaultSummary and DefaultDescription; applications may assign their own
// implementation during program initialization (before routes are
// registered), eg. to compose summaries in another language.
var (
	GenerateSummary     = DefaultSummary
	GenerateDescription = DefaultDescription
)

var (
	operationMu       sync.RWMutex
	operationRegistry = make(map[string]OperationDoc)
)

// RegisterOperation records an explicit summary/description for the operation
// identified by its HTTP method and route path. Registering the same
// operation again replaces the previous entry. The path accepts both
// gin-style ":param" and OpenAPI-style "{param}" placeholders.
func RegisterOperation(method, path string, doc OperationDoc) {
	operationMu.Lock()
	defer operationMu.Unlock()
	operationRegistry[operationKey(method, path)] = doc
}

// LookupOperation returns the OperationDoc registered for the operation
// identified by its HTTP method and route path.
func LookupOperation(method, path string) (OperationDoc, bool) {
	operationMu.RLock()
	defer operationMu.RUnlock()

	doc, ok := operationRegistry[operationKey(method, path)]
	return doc, ok
}

func operationKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + normalizeOperationPath(path)
}

// normalizeOperationPath converts gin-style ":param" segments to
// OpenAPI-style "{param}" so both spellings address the same operation.
func normalizeOperationPath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ":") && len(segment) > 1 {
			segments[i] = "{" + segment[1:] + "}"
		}
	}
	return strings.Join(segments, "/")
}

// DefaultSummary builds an operation summary from the action and the first
// line of the model doc comment, eg. "List The user record" for a model
// documented as "User is the user record.".
//
// The action is the trailing literal path segment when it follows a path
// parameter (a custom action route such as "/api/users/{id}/disable"),
// otherwise the framework verb. Without a model comment it falls back to the
// resource path segments, then to the model name, keeping the summary useful
// for models that have no doc comment yet.
func DefaultSummary(op Operation) string {
	actionSegment := trailingActionSegment(op)
	action := verbDisplay(op.Verb)
	if actionSegment != "" {
		action = titleToken(actionSegment)
	}

	if noun := firstCommentLine(op.ModelComment); noun != "" {
		return action + " " + noun
	}
	segments := operationResourceSegments(op.Path)
	// The action segment already leads the summary; do not repeat it as part
	// of the resource fallback.
	if actionSegment != "" && len(segments) > 0 && segments[len(segments)-1] == actionSegment {
		segments = segments[:len(segments)-1]
	}
	if len(segments) > 0 {
		return action + " " + strings.Join(segments, " ")
	}
	if op.ModelName != "" {
		return action + " " + op.ModelName
	}
	return action
}

// DefaultDescription uses the full model doc comment and falls back to the
// default summary when the model has no comment.
func DefaultDescription(op Operation) string {
	if comment := strings.TrimSpace(op.ModelComment); comment != "" {
		return comment
	}
	return DefaultSummary(op)
}

// trailingActionSegment returns the final literal path segment when it
// directly follows a path parameter, which is the shape of custom action
// routes such as "/api/users/{id}/disable". Nested default CRUD collection
// routes ("/api/tenants/{tenant}/users") share that shape, so the segment
// only counts as an action for operations with custom request/response
// types, and never for List: a trailing literal in a List route is the
// collection being listed, not an action.
func trailingActionSegment(op Operation) string {
	if !op.CustomTypes || op.Verb == consts.List {
		return ""
	}
	segments := strings.Split(strings.Trim(op.Path, "/"), "/")
	if len(segments) < 2 {
		return ""
	}
	last := segments[len(segments)-1]
	if last == "" || isParamSegment(last) {
		return ""
	}
	if !isParamSegment(segments[len(segments)-2]) {
		return ""
	}
	return last
}

func isParamSegment(segment string) bool {
	if strings.HasPrefix(segment, ":") && len(segment) > 1 {
		return true
	}
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")
}

// verbDisplay renders the framework verb as a summary token, eg. "Create",
// or "Batch Create" for the *_many verbs.
func verbDisplay(verb consts.HTTPVerb) string {
	if base, ok := strings.CutSuffix(string(verb), "_many"); ok {
		return "Batch " + titleToken(base)
	}
	return titleToken(string(verb))
}

// titleToken converts a snake_case or kebab-case token to space-separated
// words with each word capitalized, eg. "reset_password" -> "Reset Password".
func titleToken(token string) string {
	words := strings.FieldsFunc(token, func(r rune) bool { return r == '_' || r == '-' })
	for i, word := range words {
		first, size := utf8.DecodeRuneInString(word)
		words[i] = string(unicode.ToUpper(first)) + word[size:]
	}
	return strings.Join(words, " ")
}

// firstCommentLine returns the first line of the comment with the trailing
// sentence period removed, so it reads naturally after the action token.
func firstCommentLine(comment string) string {
	line := strings.TrimSpace(strings.SplitN(comment, "\n", 2)[0])
	line = strings.TrimSuffix(line, "。")
	line = strings.TrimSuffix(line, ".")
	return line
}

// operationResourceSegments returns the resource segments of a route path:
// the /api prefix, path parameters and empty segments are dropped.
func operationResourceSegments(path string) []string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	filtered := make([]string, 0, len(segments))
	for index, segment := range segments {
		if segment == "" || (index == 0 && segment == "api") || isParamSegment(segment) {
			continue
		}
		filtered = append(filtered, segment)
	}
	return filtered
}
