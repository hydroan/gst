package ts

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteDocEscapesWhatWouldEndOrRetagTheComment(t *testing.T) {
	// The first two cases are the examples of the writeDoc doc comment, the
	// second with the indent of a property.
	var b strings.Builder
	writeDoc(&b, "", "ends with */ inside")
	require.Equal(t, "/** ends with *\\/ inside */\n", b.String())

	b.Reset()
	writeDoc(&b, "  ", "Title is the display title.\n\n@internal must not read as a tag")
	require.Equal(t, "  /**\n   * Title is the display title.\n   *\n   * \\@internal must not read as a tag\n   */\n", b.String())

	b.Reset()
	writeDoc(&b, "", "")
	require.Empty(t, b.String())
}

func TestPropertyNameQuotesWhatIsNotAnIdentifier(t *testing.T) {
	tests := map[string]string{
		"title":      "title",
		"$ref":       "$ref",
		"trace_id":   "trace_id",
		"-":          `"-"`,
		"created at": `"created at"`,
		"2fa":        `"2fa"`,
	}
	for key, want := range tests {
		require.Equal(t, want, propertyName(key), "key %q", key)
	}
}

func TestQuoteStringLeavesTextReadable(t *testing.T) {
	// encoding/json escapes <, > and & by default, which would turn a readable
	// literal into escape sequences. The first case is the example of the
	// quoteString doc comment.
	require.Equal(t, `"a<b&c>d"`, quoteString("a<b&c>d"))
	require.Equal(t, `"quote\"inside"`, quoteString(`quote"inside`))
	require.Equal(t, `"line\nbreak"`, quoteString("line\nbreak"))
}

func TestArrayOfParenthesizesAUnion(t *testing.T) {
	// The first and last cases are the examples of the arrayOf doc comment.
	require.Equal(t, "string[]", arrayOf("string"))
	require.Equal(t, "Record[]", arrayOf("Record"))
	require.Equal(t, `(Status | "")[]`, arrayOf(`Status | ""`))
	require.Equal(t, "(Record | null)[]", arrayOf("Record | null"))
}
