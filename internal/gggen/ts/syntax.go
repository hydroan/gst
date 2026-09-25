package ts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// This file holds the TypeScript syntax the declarations are written in: doc
// comments, property names, string literals, array types and the names
// TypeScript reserves.

// writeDoc writes doc as a JSDoc comment, every line indented by indent: on
// one line for a single line of doc, and as a block otherwise. A "*/" would
// end the comment early and a line starting with @ would read as a JSDoc tag,
// so both are escaped. The doc "ends with */ inside" is written as
//
//	/** ends with *\/ inside */
//
// and the doc "Title is the display title.", a blank line and "@internal must
// not read as a tag" as
//
//	/**
//	 * Title is the display title.
//	 *
//	 * \@internal must not read as a tag
//	 */
func writeDoc(b *strings.Builder, indent, doc string) {
	if doc == "" {
		return
	}
	lines := strings.Split(strings.ReplaceAll(doc, "*/", `*\/`), "\n")
	for i, line := range lines {
		if trimmed := strings.TrimLeft(line, " \t"); strings.HasPrefix(trimmed, "@") {
			lines[i] = line[:len(line)-len(trimmed)] + `\` + trimmed
		}
	}
	if len(lines) == 1 {
		fmt.Fprintf(b, "%s/** %s */\n", indent, lines[0])
		return
	}
	fmt.Fprintf(b, "%s/**\n", indent)
	for _, line := range lines {
		if line == "" {
			fmt.Fprintf(b, "%s *\n", indent)
			continue
		}
		fmt.Fprintf(b, "%s * %s\n", indent, line)
	}
	fmt.Fprintf(b, "%s */\n", indent)
}

// identifier matches a property name TypeScript accepts unquoted.
var identifier = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

// propertyName renders a JSON key as a property name, quoted unless it is a
// plain identifier: title and trace_id stay as they are, while - and
// created at become "-" and "created at".
func propertyName(key string) string {
	if identifier.MatchString(key) {
		return key
	}
	return quoteString(key)
}

// quoteString renders s as a TypeScript string literal, as "a<b&c>d" for
// a<b&c>d. JSON string syntax is valid TypeScript; HTML escaping is turned off
// to keep the literal readable, where encoding/json would write
// "a\u003cb\u0026c\u003ed".
func quoteString(s string) string {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(s); err != nil {
		return strconv.Quote(s)
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// arrayOf renders an array of elem, parenthesizing a union: string[] for
// string and (Record | null)[] for Record | null.
func arrayOf(elem string) string {
	if strings.Contains(elem, " | ") {
		return "(" + elem + ")[]"
	}
	return elem + "[]"
}

// reservedTypeNames are the names TypeScript refuses for a type: its reserved
// words and predefined type names. Go allows some of them as unexported type
// names.
var reservedTypeNames = map[string]bool{
	"any": true, "bigint": true, "boolean": true, "break": true, "case": true, "catch": true,
	"class": true, "const": true, "continue": true, "debugger": true, "default": true,
	"delete": true, "do": true, "else": true, "enum": true, "export": true, "extends": true,
	"false": true, "finally": true, "for": true, "function": true, "if": true,
	"implements": true, "import": true, "in": true, "instanceof": true, "interface": true,
	"let": true, "never": true, "new": true, "null": true, "number": true, "object": true,
	"package": true, "private": true, "protected": true, "public": true, "return": true,
	"static": true, "string": true, "super": true, "switch": true, "symbol": true,
	"this": true, "throw": true, "true": true, "try": true, "typeof": true,
	"undefined": true, "unknown": true, "var": true, "void": true, "while": true,
	"with": true, "yield": true,
}
