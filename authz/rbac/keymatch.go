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
// A template spells a path, with two things standing for more than themselves:
// {name} matches one segment, and /* matches the rest of the path, separators
// included. Everything else is the text it looks like.
//
// That last part is where the template language and Casbin's keyMatch3 part
// company. keyMatch3 substitutes those two forms and hands the whole result to
// the regexp engine, which leaves every other metacharacter live: "/api/.*"
// stored as one route's object reaches every route in the tenant, and a
// template that does not compile fails every request that reaches it, because a
// denial evaluates the whole policy set. Neither is something a policy asked
// for, and neither is visible in the policy that causes it.
//
// The template is compiled once and kept, rather than rebuilt per call.
// keyMatch3 rebuilds it every time, and alone among its neighbours: it reaches
// the regexp through util.RegexMatch, which calls regexp.MatchString, while
// keyMatch2, keyMatch4 and keyMatch5 all resolve their pattern through the
// compiled-pattern cache that package keeps. The matcher evaluates this
// function once per stored policy per request, so the rebuild dominated the
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
//
// The expression is assembled rather than parsed: the placeholders are located,
// and everything between them is quoted. Only the two forms the template
// language defines survive as anything wider than the text they spell, so no
// stored object can widen the policy it belongs to.
//
// Quoting can only narrow what a template matches, so a policy written while
// the whole template was a regular expression never allows more than it did.
func compilePathTemplate(template string) compiledPathTemplate {
	if cached, hit := pathTemplateCache.Load(template); hit {
		if compiled, ok := pathTemplateOf(cached); ok {
			return compiled
		}
	}

	var pattern strings.Builder
	pattern.WriteString("^")
	quoted := 0
	for _, placeholder := range pathTemplatePlaceholder.FindAllStringIndex(template, -1) {
		pattern.WriteString(quotePathTemplate(template[quoted:placeholder[0]]))
		// A placeholder stops at a separator, which is what distinguishes it
		// from the wildcard.
		pattern.WriteString("[^/]+")
		quoted = placeholder[1]
	}
	pattern.WriteString(quotePathTemplate(template[quoted:]))
	pattern.WriteString("$")

	compiled := compiledPathTemplate{}
	expression, err := regexp.Compile(pattern.String())
	if err != nil {
		// Quoting rules out a syntax error, so what reaches here is a template
		// that is not valid UTF-8: QuoteMeta passes those bytes through
		// unescaped and the parser rejects them. It is reported rather than
		// panicked, which is what Casbin does with the far larger class of
		// failures it lets through, at the price of a stack dump per request
		// and no mention of the template that caused it.
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

// quotePathTemplate quotes the part of a template that holds no placeholder,
// leaving the wildcard as the one form that keeps a meaning of its own.
//
// The wildcard is recognized wherever it appears rather than only at the end,
// because that is where keyMatch3 recognized it, and narrowing it to a suffix
// would take away grants that are written and working.
func quotePathTemplate(literal string) string {
	spans := strings.Split(literal, "/*")
	for i, span := range spans {
		spans[i] = regexp.QuoteMeta(span)
	}
	// The wildcard spans separators, which is what distinguishes it from a
	// placeholder.
	return strings.Join(spans, "/.*")
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
