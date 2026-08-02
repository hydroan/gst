package rbac

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/casbin/casbin/v3/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// keyMatch3 reports what Casbin's own keyMatch3 answers for the same
// arguments, and whether it left through a panic instead of answering.
//
// It builds its expression with MustCompile, so a template that cannot compile
// panics rather than reporting anything. That is the behavior pathMatch
// replaces, and telling the two exits apart is what lets a comparison assert
// they agree on every input rather than only on the ones that answer.
func keyMatch3(path string, template string) (matched bool, panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	return util.KeyMatch3(path, template), false
}

// isPlainTemplate reports whether everything in template outside a placeholder
// and a wildcard is text the regexp engine would have read the same way.
//
// Those are the templates the framework itself writes, from router.Routes and
// from menu route bindings, and the two implementations have to agree on every
// one of them: narrowing the reading of a metacharacter is the point, and
// changing anything else would be a regression.
// The two forms are removed in the order keyMatch3 substitutes them: the
// wildcard is recognized in the template as written, before removing a
// placeholder can leave behind a "/*" the template never contained. Reversing
// the two calls reports "/{0}*" as plain, which it is not — keyMatch3 turns it
// into the nested repetition "/[^/]+*" and cannot compile it at all.
func isPlainTemplate(template string) bool {
	stripped := strings.ReplaceAll(template, "/*", "/")
	stripped = pathTemplatePlaceholder.ReplaceAllString(stripped, "")
	return regexp.QuoteMeta(stripped) == stripped
}

// TestPathMatchMatchesTheTemplateLanguage pins the semantics of a template:
// {name} stands for one segment, /* for the rest of the path, and everything
// else for the text it spells.
func TestPathMatchMatchesTheTemplateLanguage(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		template string
		matched  bool
	}{
		{"exact", "/api/items", "/api/items", true},
		{"different path", "/api/items", "/api/others", false},
		{"trailing slash is not ignored", "/api/items/", "/api/items", false},
		{"matching is case sensitive", "/API/items", "/api/items", false},

		{"placeholder takes one segment", "/api/items/1", "/api/items/{id}", true},
		{"placeholder does not span segments", "/api/items/1/2", "/api/items/{id}", false},
		{"placeholder needs a segment", "/api/items/", "/api/items/{id}", false},
		{"several placeholders", "/api/a/children/b", "/api/{id}/children/{child}", true},
		{"adjacent placeholders", "/api/ab", "/api/{a}{b}", true},
		{"leading placeholder", "x/api", "{id}/api", true},
		{"placeholder matches multi-byte runes", "/api/名前", "/api/{name}", true},
		{"empty braces are literal", "/api/{}", "/api/{}", true},
		{"braces holding a slash are literal", "/api/{a/b}", "/api/{a/b}", true},

		{"suffix wildcard spans segments", "/api/a/b/c", "/api/*", true},
		{"wildcard mid template", "/api/x/items", "/api/*/items", true},
		{"suffix wildcard still needs the prefix", "/other/a", "/api/*", false},

		{"empty template matches empty path", "", "", true},
		{"empty template rejects a path", "/x", "", false},

		{"a dot is a dot", "/api/a.b", "/api/a.b", true},
		{"a dot matches nothing else", "/api/axb", "/api/a.b", false},
		{"a bracket is a bracket", "/api/[", "/api/[", true},
		// keyMatch3 compiled this one to an escaped dollar and matched "$",
		// a path the template does not spell. Found by the fuzzer.
		{"a backslash is a backslash", "\\", "\\", true},
		{"a backslash reaches nothing else", "$", "\\", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			matched, err := pathMatch(c.path, c.template)
			require.NoError(t, err)
			assert.Equal(t, c.matched, matched, "path %q against template %q", c.path, c.template)

			// Every case here is a template the framework itself could write,
			// so the reading must not have moved.
			if isPlainTemplate(c.template) {
				expected, panicked := keyMatch3(c.path, c.template)
				require.False(t, panicked, "keyMatch3 must answer for a plain template")
				assert.Equal(t, expected, matched, "a plain template must read as it always did")
			}
		})
	}
}

// TestPathMatchTreatsMetacharactersAsText covers what the template language
// stopped granting.
//
// keyMatch3 hands the whole template to the regexp engine, so a metacharacter
// stored as a route's object reaches paths that route never named — and a
// template that does not compile fails every request that reaches it, because
// a denial evaluates the whole policy set. Neither is expressible any more.
func TestPathMatchTreatsMetacharactersAsText(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		template string
	}{
		{"wildcard regexp no longer reaches every route", "/api/authz/roles", "/api/.*"},
		{"any-character no longer spans a segment", "/api/axb", "/api/a.b"},
		{"alternation no longer offers a choice", "/api/b", "/api/(a|b)"},
		{"repetition no longer applies", "/api/aaa", "/api/a+"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			allowed, panicked := keyMatch3(c.path, c.template)
			require.False(t, panicked)
			require.True(t, allowed, "this case is only interesting if keyMatch3 allowed it")

			matched, err := pathMatch(c.path, c.template)
			require.NoError(t, err)
			assert.False(t, matched, "template %q must no longer reach %q", c.template, c.path)
		})
	}
}

// TestPathMatchSurvivesATemplateKeyMatch3CouldNotCompile covers the other half:
// a template that made keyMatch3 panic is now ordinary text, so it answers for
// itself and stops taking every other decision down with it.
func TestPathMatchSurvivesATemplateKeyMatch3CouldNotCompile(t *testing.T) {
	const template = "/api/items/["
	t.Cleanup(func() { pathTemplateCache.Delete(template) })

	_, panicked := keyMatch3("/api/items/1", template)
	require.True(t, panicked, "this case is only interesting if keyMatch3 could not compile it")

	matched, err := pathMatch("/api/items/1", template)
	require.NoError(t, err)
	assert.False(t, matched)

	matched, err = pathMatch(template, template)
	require.NoError(t, err)
	assert.True(t, matched, "the template must still match the path it spells")
}

// FuzzPathMatchReachesOnlyWhatTheTemplateSpells pins the two properties the
// change exists for.
//
// A template that uses neither of the two forms reaches exactly one path: the
// one it spells. That is the whole of it — whatever characters it holds, a
// stored object cannot reach past the route it names.
//
// And for a template the framework itself writes, the reading must not have
// moved at all, so it still has to agree with keyMatch3 exactly.
//
// The comparison is deliberately not "never matches more than keyMatch3 did".
// That is false, and the fuzzer says so: the template `\` used to compile to
// `^\$`, an escaped dollar, and matched the path `$` — a path the template does
// not spell and never named. Reading it as text matches `\` instead. The old
// reading was not a smaller set, it was a different one.
//
// The cached template is dropped afterwards because the cache is keyed by
// template and bounded, in production, by the stored policy set; a fuzz run
// invents templates without that bound and would otherwise grow it without
// limit.
func FuzzPathMatchReachesOnlyWhatTheTemplateSpells(f *testing.F) {
	for _, seed := range [][2]string{
		{"/api/items", "/api/items"},
		{"/api/items/1", "/api/items/{id}"},
		{"/api/a/b", "/api/*"},
		{"/api/x", "/api/["},
		{"/api/{}", "/api/{}"},
		{"", ""},
		{"/api/名前", "/api/{name}"},
		{"/api/xbc", "/api/.bc"},
		{"/api/authz/roles", "/api/.*"},
		// Both found by the fuzzer, and both about the order the two forms are
		// recognized in rather than about matching: see isPlainTemplate.
		{"\\", "\\"},
		{"/x*", "/{0}*"},
	} {
		f.Add(seed[0], seed[1])
	}

	f.Fuzz(func(t *testing.T, path string, template string) {
		defer pathTemplateCache.Delete(template)

		matched, err := pathMatch(path, template)
		if err != nil {
			// A template this rejects reaches nothing at all.
			return
		}

		if !pathTemplatePlaceholder.MatchString(template) && !strings.Contains(template, "/*") {
			require.Equal(t, path == template, matched,
				"template %q must reach the path it spells and no other, but answered %v for %q",
				template, matched, path)
			return
		}

		if isPlainTemplate(template) {
			allowed, panicked := keyMatch3(path, template)
			require.False(t, panicked, "keyMatch3 must answer for the plain template %q", template)
			require.Equal(t, allowed, matched,
				"plain template %q must read as it always did against %q", template, path)
		}
	})
}

// TestPathMatchReportsTemplatesItCannotCompile covers the one input quoting
// does not rescue: QuoteMeta passes bytes that are not valid UTF-8 through
// unescaped, and the parser rejects them. A policy table can hold such a row,
// so the failure is reported with the template named rather than panicked the
// way Casbin does.
func TestPathMatchReportsTemplatesItCannotCompile(t *testing.T) {
	const template = "/api/unusable/\xff"
	t.Cleanup(func() { pathTemplateCache.Delete(template) })

	matched, err := pathMatch("/api/unusable/1", template)
	require.Error(t, err)
	assert.False(t, matched, "a template that cannot compile must not allow anything")
	assert.Contains(t, err.Error(), "unusable path template",
		"the error must name the template so it can be repaired")
}

// TestPathMatchCompilesEachTemplateOnce covers the caching itself, for both
// outcomes. A template reaching this function does so once per stored policy
// per request, so recompiling either outcome is the cost the cache exists to
// remove.
func TestPathMatchCompilesEachTemplateOnce(t *testing.T) {
	t.Run("usable template", func(t *testing.T) {
		const template = "/api/compiled-once/{id}"
		t.Cleanup(func() { pathTemplateCache.Delete(template) })

		first, second := compilePathTemplate(template), compilePathTemplate(template)
		require.NoError(t, first.err)
		assert.Same(t, first.expression, second.expression,
			"the second lookup must reuse the compiled expression rather than build another")
	})

	t.Run("unusable template", func(t *testing.T) {
		const template = "/api/compiled-once/\xff"
		t.Cleanup(func() { pathTemplateCache.Delete(template) })

		first, second := compilePathTemplate(template), compilePathTemplate(template)
		require.Error(t, first.err)
		assert.Same(t, first.err, second.err,
			"a failed compile must be remembered too, or it repeats on every request")
	})
}

// TestPathMatchIsSafeForConcurrentUse covers the read path as it actually runs:
// every request evaluates the matcher against the same templates at once, so
// the first compile of a template happens under contention.
func TestPathMatchIsSafeForConcurrentUse(t *testing.T) {
	const template = "/api/concurrent/{id}"
	t.Cleanup(func() { pathTemplateCache.Delete(template) })

	const goroutines = 64
	// Each goroutine records what it saw at its own index and the assertions
	// run once the group has finished: a failed assertion has to stop the test
	// goroutine, which it cannot do from one it did not start on.
	type attempt struct {
		expression *regexp.Regexp
		matched    bool
		err        error
	}
	attempts := make([]attempt, goroutines)

	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Go(func() {
			matched, err := pathMatch("/api/concurrent/"+strconv.Itoa(i), template)
			attempts[i] = attempt{compilePathTemplate(template).expression, matched, err}
		})
	}
	wg.Wait()

	for i, got := range attempts {
		require.NoError(t, got.err, "goroutine %d", i)
		assert.True(t, got.matched, "goroutine %d", i)
		assert.Same(t, attempts[0].expression, got.expression,
			"every caller must end up on the one compiled expression")
	}
}

// TestPathMatchFuncRejectsUnusableArguments covers the adapter the matcher
// calls. The matcher passes model tokens straight through, so a model naming a
// token that does not exist arrives here as a non-string.
func TestPathMatchFuncRejectsUnusableArguments(t *testing.T) {
	cases := []struct {
		name string
		args []any
	}{
		{"too few arguments", []any{"/api/items"}},
		{"too many arguments", []any{"/api/items", "/api/items", "extra"}},
		{"path is not a string", []any{1, "/api/items"}},
		{"template is not a string", []any{"/api/items", 1}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			matched, err := pathMatchFunc(c.args...)
			require.Error(t, err)
			assert.Equal(t, false, matched, "a rejected call must not allow anything")
			assert.Contains(t, err.Error(), matcherFuncPathMatch,
				"the error must name the function so the model can be repaired")
		})
	}
}

// BenchmarkPathMatch and BenchmarkKeyMatch3 measure one matcher call, which the
// enforcer makes once per stored policy per request.
func BenchmarkPathMatch(b *testing.B) {
	const path, template = "/api/items/300/children/7", "/api/items/{id}/children/{child}"
	b.Cleanup(func() { pathTemplateCache.Delete(template) })

	b.ReportAllocs()
	for range b.N {
		matched, err := pathMatch(path, template)
		if err != nil || !matched {
			b.Fatalf("matched=%v err=%v", matched, err)
		}
	}
}

func BenchmarkKeyMatch3(b *testing.B) {
	const path, template = "/api/items/300/children/7", "/api/items/{id}/children/{child}"

	b.ReportAllocs()
	for range b.N {
		if !util.KeyMatch3(path, template) {
			b.Fatal("expected a match")
		}
	}
}
