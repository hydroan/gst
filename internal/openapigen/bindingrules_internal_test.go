package openapigen

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/go-playground/validator/v10"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/stretchr/testify/require"
)

// TestBindingRulesAreTheValidatorsOwn keeps what the document reads of a
// binding tag in step with the validator behind the tag, go-playground's.
// Every rule name the document knows is one the validator takes: it panics
// on a name it does not know, which is how a rule it renamed would show
// here. And what the document states of a rule is what the validator
// enforces: of two values either side of the bound, the validator and the
// schema keyword let the same one through and refuse the same one. The one
// name the validator has to be told of is the framework's own readonly (see
// modelschema.RuleReadOnly).
func TestBindingRulesAreTheValidatorsOwn(t *testing.T) {
	validate := validator.New()
	require.Panics(t, func() { _ = validate.Var("a", "no_such_rule") })
	decoded := func(value any) any {
		t.Helper()
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		var out any
		require.NoError(t, json.Unmarshal(encoded, &out))
		return out
	}

	for _, tt := range []struct {
		rule            string
		inside, outside any
	}{
		{ruleRequired, "a", ""},
		{ruleMin + "=2", "ab", "a"},
		{ruleMin + "=2", 2, 1},
		{ruleMin + "=2", []int{1, 2}, []int{1}},
		{ruleMin + "=2", map[string]int{"a": 1, "b": 2}, map[string]int{"a": 1}},
		{ruleMax + "=2", "ab", "abc"},
		{ruleMax + "=2", 2, 3},
		{ruleMax + "=2", []int{1, 2}, []int{1, 2, 3}},
		{ruleMax + "=2", map[string]int{"a": 1, "b": 2}, map[string]int{"a": 1, "b": 2, "c": 3}},
		{ruleLen + "=2", "ab", "abc"},
		{ruleLen + "=2", []int{1, 2}, []int{1}},
		{ruleGTE + "=2", "ab", "a"},
		{ruleGTE + "=2", 2.0, 1.5},
		{ruleGT + "=2", "abc", "ab"},
		{ruleGT + "=2", 2.5, 2.0},
		{ruleGT + "=2", []int{1, 2, 3}, []int{1, 2}},
		{ruleLTE + "=2", "ab", "abc"},
		{ruleLTE + "=2", 2, 3},
		{ruleLT + "=2", "a", "ab"},
		{ruleLT + "=2", 1.5, 2.0},
		{ruleLT + "=2", []int{1}, []int{1, 2}},
		{ruleOneOf + "=draft 'in review' done", "in review", "review"},
	} {
		ref, err := openapi3gen.NewSchemaRefForValue(tt.inside, nil)
		require.NoError(t, err)
		require.True(t, stateBindingRule(ref.Value, reflect.TypeOf(tt.inside), tt.rule), "%s on %T is stated by a keyword", tt.rule, tt.inside)

		require.NoError(t, validate.Var(tt.inside, tt.rule), "the validator lets %v through %s", tt.inside, tt.rule)
		require.NoError(t, ref.Value.VisitJSON(decoded(tt.inside)), "the keyword of %s lets %v through", tt.rule, tt.inside)
		require.Error(t, validate.Var(tt.outside, tt.rule), "the validator refuses %v under %s", tt.outside, tt.rule)
		require.Error(t, ref.Value.VisitJSON(decoded(tt.outside)), "the keyword of %s refuses %v", tt.rule, tt.outside)
	}

	// The values of a number are listed as the numbers the rule names.
	levels, err := openapi3gen.NewSchemaRefForValue(1, nil)
	require.NoError(t, err)
	require.True(t, stateBindingRule(levels.Value, reflect.TypeFor[int](), ruleOneOf+"=1 2 3"))
	require.Equal(t, []any{int64(1), int64(2), int64(3)}, levels.Value.Enum)
	require.NoError(t, validate.Var(2, ruleOneOf+"=1 2 3"))
	require.Error(t, validate.Var(4, ruleOneOf+"=1 2 3"))

	// A rule holding a string to a format: a value of the format passes it
	// and a plain word does not. Every format the document knows has a
	// sample here.
	samples := map[string]string{
		"email":            "someone@example.com",
		"url":              "https://example.com/a",
		"uri":              "urn:example:a",
		"uuid":             "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
		"uuid3":            "a3bb189e-8bf9-3888-9912-ace4e6543002",
		"uuid4":            "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"uuid5":            "886313e1-3b8a-5372-9b90-0c9aee199e5d",
		"ipv4":             "192.0.2.1",
		"ipv6":             "2001:db8::1",
		"hostname":         "host-1",
		"hostname_rfc1123": "1host.example",
	}
	require.ElementsMatch(t, slices.Collect(maps.Keys(bindingRuleFormats)), slices.Collect(maps.Keys(samples)))
	for rule, sample := range samples {
		require.NoError(t, validate.Var(sample, rule), "%s passes %s", sample, rule)
		require.Error(t, validate.Var("not one!", rule), "a plain word passes %s", rule)
	}

	// dive hands the rules after it to the elements, and keys and endkeys
	// the ones between them to the keys of a map.
	require.NoError(t, validate.Var([]string{"a"}, ruleDive+","+ruleRequired))
	require.Error(t, validate.Var([]string{""}, ruleDive+","+ruleRequired))
	keyed := strings.Join([]string{ruleDive, ruleKeys, ruleMax + "=1", ruleEndKeys, ruleMax + "=2"}, ",")
	require.NoError(t, validate.Var(map[string]string{"k": "vv"}, keyed))
	require.Error(t, validate.Var(map[string]string{"kk": "vv"}, keyed), "the rules between keys and endkeys are the keys'")
	require.Error(t, validate.Var(map[string]string{"k": "vvv"}, keyed), "the rules after endkeys are the values'")

	// The rules the document says nothing of refuse nothing by themselves.
	for _, rule := range silentBindingRules {
		if rule == modelschema.RuleReadOnly {
			continue
		}
		require.NoError(t, validate.Var("", rule), "%q refuses a value by itself", rule)
	}
}
