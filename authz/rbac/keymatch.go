package rbac

import (
	"regexp"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
)

// matcherFuncPathMatch is the name modelData's matcher calls pathMatch by.
//
// It is deliberately not Casbin's keyMatch3, and could not be: FunctionMap
// stores through LoadOrStore, so a built-in name can never be replaced. The
// matcher has to name this function for the compiled templates to be used at
// all.
const matcherFuncPathMatch = "pathMatch"

// pathTemplatePlaceholder matches the {name} placeholder that stands for one
// path segment, which is the placeholder syntax Casbin's keyMatch3 defines.
var pathTemplatePlaceholder = regexp.MustCompile(`\{[^/]+\}`)

// compiledPathTemplate is what one path template compiles to.
//
// A failure is kept alongside a success. A template that cannot compile is
// still reached once per stored policy per request, so discarding the failure
// would repeat the compile that already failed, on every request, for as long
// as the template stays in the policy set.
type compiledPathTemplate struct {
	expression *regexp.Regexp
	err        error
}

// pathTemplateCache holds the compiled form of every template seen so far.
//
// Only the template is ever a key. Templates come from the stored policy set,
// which is what bounds the cache; the request path is matched against the
// compiled expression and never becomes a key, so nothing a client sends can
// grow it.
var pathTemplateCache sync.Map

// pathMatch reports whether path matches template.
//
// The semantics are Casbin's keyMatch3: {name} matches one segment, /* matches
// any suffix, and what remains is a regular expression anchored at both ends.
// What differs is that the expression is compiled once per template instead of
// once per call.
//
// Casbin rebuilds it every call, and only for this one matcher: keyMatch3
// reaches the regexp through util.RegexMatch, which calls regexp.MatchString,
// while keyMatch2, keyMatch4 and keyMatch5 all resolve their pattern through
// the compiled-pattern cache that package keeps. The matcher evaluates this
// function once per stored policy per request, so the rebuild dominates the
// cost of a decision rather than adding to it.
func pathMatch(path string, template string) (bool, error) {
	compiled := compilePathTemplate(template)
	if compiled.err != nil {
		return false, compiled.err
	}
	return compiled.expression.MatchString(path), nil
}

// pathTemplateOf reads a cache entry back as the type only compilePathTemplate
// ever stores.
//
// The assertion is checked even though nothing else writes the cache: an
// unchecked one answers a mismatch with a zero value carrying neither an
// expression nor an error, which nothing downstream could tell apart from a
// template that compiled.
func pathTemplateOf(cached any) (compiledPathTemplate, bool) {
	compiled, ok := cached.(compiledPathTemplate)
	return compiled, ok
}

// compilePathTemplate turns a path template into an anchored expression,
// returning the cached one when the template has been seen before.
func compilePathTemplate(template string) compiledPathTemplate {
	if cached, hit := pathTemplateCache.Load(template); hit {
		if compiled, ok := pathTemplateOf(cached); ok {
			return compiled
		}
	}

	// The two rewrites and the anchoring reproduce keyMatch3 exactly. Casbin
	// writes the placeholder replacement as "$1[^/]+$2", but its expression
	// declares no capture group, so both references expand to nothing and the
	// literal below is what it substitutes.
	pattern := strings.ReplaceAll(template, "/*", "/.*")
	pattern = pathTemplatePlaceholder.ReplaceAllString(pattern, "[^/]+")

	compiled := compiledPathTemplate{}
	expression, err := regexp.Compile("^" + pattern + "$")
	if err != nil {
		// Reported rather than panicked. Casbin panics here and leaves the
		// enforcer to recover, which costs a stack dump on every request and
		// drops the one detail worth having: which template failed to compile.
		compiled.err = errors.Wrapf(err, "rbac: policy holds an unusable path template %q", template)
	} else {
		compiled.expression = expression
	}

	// The stored value is returned rather than the one just built, so that two
	// callers compiling the same template concurrently still leave every
	// matcher evaluation sharing one expression.
	actual, _ := pathTemplateCache.LoadOrStore(template, compiled)
	if stored, ok := pathTemplateOf(actual); ok {
		return stored
	}
	return compiled
}

// pathMatchFunc adapts pathMatch to the signature the matcher calls it with.
//
// The argument checks mirror Casbin's own: the matcher passes tokens straight
// through, so a model naming the wrong token reaches this function with a value
// that is not a string, and reporting that is more use than a type assertion
// panic recovered several frames away.
func pathMatchFunc(args ...any) (any, error) {
	if len(args) != 2 {
		return false, errors.Newf("%s: expected 2 arguments, got %d", matcherFuncPathMatch, len(args))
	}
	path, ok := args[0].(string)
	if !ok {
		return false, errors.Newf("%s: argument 1 must be a string, got %T", matcherFuncPathMatch, args[0])
	}
	template, ok := args[1].(string)
	if !ok {
		return false, errors.Newf("%s: argument 2 must be a string, got %T", matcherFuncPathMatch, args[1])
	}
	return pathMatch(path, template)
}
