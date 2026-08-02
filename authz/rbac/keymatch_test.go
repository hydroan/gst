package rbac

import (
	"regexp"
	"strconv"
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

// requirePathMatchAgreesWithKeyMatch3 asserts the two answer alike: the same
// verdict where Casbin answers, and an error exactly where Casbin panics.
func requirePathMatchAgreesWithKeyMatch3(tb testing.TB, path string, template string) bool {
	tb.Helper()

	expected, panicked := keyMatch3(path, template)
	matched, err := pathMatch(path, template)
	if panicked {
		require.Error(tb, err,
			"keyMatch3 panics on template %q, so pathMatch has to report it", template)
		return false
	}
	require.NoError(tb, err, "keyMatch3 answers for template %q, so pathMatch must too", template)
	require.Equal(tb, expected, matched, "path %q against template %q", path, template)
	return matched
}

// TestPathMatchAgreesWithKeyMatch3 pins the semantics pathMatch inherits.
//
// Every case asserts the expected verdict as well as agreement with Casbin, so
// the table documents the matching rules rather than only asserting that two
// implementations are equally wrong.
func TestPathMatchAgreesWithKeyMatch3(t *testing.T) {
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

		// Not a quirk of this implementation: the template is a regular
		// expression, so a metacharacter stored in a policy widens what that
		// policy allows. Recorded here because it is inherited behavior, and
		// changing it belongs to whatever validates templates on the way in.
		{"metacharacters keep regexp meaning", "/api/xbc", "/api/.bc", true},
		{"alternation keeps regexp meaning", "/api/b", "/api/(a|b)", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.matched, requirePathMatchAgreesWithKeyMatch3(t, c.path, c.template))
		})
	}
}

// FuzzPathMatchAgreesWithKeyMatch3 searches for any input where the compiled
// template answers differently from Casbin rebuilding it.
//
// The cached template is dropped afterwards because the cache is keyed by
// template and bounded, in production, by the stored policy set; a fuzz run
// invents templates without that bound and would otherwise grow it without
// limit.
func FuzzPathMatchAgreesWithKeyMatch3(f *testing.F) {
	for _, seed := range [][2]string{
		{"/api/items", "/api/items"},
		{"/api/items/1", "/api/items/{id}"},
		{"/api/a/b", "/api/*"},
		{"/api/x", "/api/["},
		{"/api/{}", "/api/{}"},
		{"", ""},
		{"/api/名前", "/api/{name}"},
		{"/api/xbc", "/api/.bc"},
	} {
		f.Add(seed[0], seed[1])
	}

	f.Fuzz(func(t *testing.T, path string, template string) {
		defer pathTemplateCache.Delete(template)
		requirePathMatchAgreesWithKeyMatch3(t, path, template)
	})
}

// TestPathMatchReportsUnusableTemplatesInsteadOfPanicking covers a policy
// holding a template that cannot compile. Casbin panics and leaves the
// enforcer to recover, which reports that something failed without saying
// which template did.
func TestPathMatchReportsUnusableTemplatesInsteadOfPanicking(t *testing.T) {
	const template = "/api/unusable/["

	matched, err := pathMatch("/api/unusable/1", template)
	require.Error(t, err)
	assert.False(t, matched, "a template that cannot compile must not allow anything")
	assert.Contains(t, err.Error(), template, "the error must name the template so it can be repaired")
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
		const template = "/api/compiled-once/["
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
