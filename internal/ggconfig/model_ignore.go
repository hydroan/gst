package ggconfig

import (
	"go/token"

	"github.com/cockroachdb/errors"
	"gopkg.in/yaml.v3"
)

// ModelIgnoreRules is the parsed gen.models.ignore mapping, one ModelRule
// per model name entry.
type ModelIgnoreRules []ModelRule

// ModelRule is a single parsed model registration ignore entry.
type ModelRule struct {
	// Name is the Go struct name of the model, e.g. "Profile".
	Name string

	// From restricts the rule to models whose file path lives under this
	// directory prefix (e.g. "model/iam"). Empty means the rule applies to
	// every model with a matching name. It protects a project's own model
	// of the same name declared elsewhere.
	From string

	// Raw preserves the original entry for error and log output.
	Raw string
}

// UnmarshalYAML parses the model-name mapping form of gen.models.ignore:
//
//	ignore:
//	  Profile:
//	    from: model/iam
//	  Widget:
//
// Each key is the Go struct name of a model whose generated model.Register
// call gg gen must skip. The optional object value adds "from", restricting
// the rule to models declared under that directory so a project's own model
// of the same name elsewhere keeps registering.
func (r *ModelIgnoreRules) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return errors.New("gen.models.ignore must be a mapping of model name to an optional {from} object")
	}

	rules := make([]ModelRule, 0, len(value.Content)/2)
	seenNames := make(map[string]struct{}, len(value.Content)/2)
	for i := 0; i+1 < len(value.Content); i += 2 {
		var name string
		if err := value.Content[i].Decode(&name); err != nil {
			return errors.Wrap(err, "model name must be a string")
		}
		if !token.IsIdentifier(name) || !token.IsExported(name) {
			return errors.Newf("model %q is not an exported Go identifier", name)
		}
		if _, ok := seenNames[name]; ok {
			return errors.Newf("duplicate model %q", name)
		}
		seenNames[name] = struct{}{}

		from, err := decodeModelIgnoreRuleValue(name, value.Content[i+1])
		if err != nil {
			return err
		}
		rules = append(rules, ModelRule{Name: name, From: from, Raw: name})
	}

	*r = rules
	return nil
}

// decodeModelIgnoreRuleValue decodes one ignore entry value: either empty
// (no restriction) or a mapping with an optional "from" directory prefix.
// Unknown mapping keys are rejected to keep gst.yaml parsing strict.
func decodeModelIgnoreRuleValue(name string, value *yaml.Node) (string, error) {
	if value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
		return "", nil
	}
	if value.Kind != yaml.MappingNode {
		return "", errors.Newf("model %q must map to an optional {from} object", name)
	}

	var from string
	for i := 0; i+1 < len(value.Content); i += 2 {
		var key string
		if err := value.Content[i].Decode(&key); err != nil {
			return "", errors.Wrapf(err, "invalid key in model %q", name)
		}
		switch key {
		case "from":
			if err := value.Content[i+1].Decode(&from); err != nil {
				return "", errors.Wrapf(err, "from of model %q must be a string", name)
			}
			normalized, err := normalizeFromDir(from)
			if err != nil {
				return "", errors.Wrapf(err, "model %q", name)
			}
			from = normalized
		default:
			return "", errors.Newf("model %q has unknown field %q, want from", name, key)
		}
	}
	return from, nil
}

// MatchesSource reports whether the rule applies to a model declared in
// the given model file. Rules without a From prefix apply to every model.
func (r ModelRule) MatchesSource(modelFilePath string) bool {
	return underPath(r.From, modelFilePath)
}
