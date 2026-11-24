// Package answer answers with sentences, held by a constant or not.
package answer

import (
	"fmt"
	"net/http"

	"example.com/module"
	"example.com/module/internal/i18n"
	"example.com/module/internal/shared"
	"example.com/module/internal/gst"
)

// reported answers with what the check reports, one a line: four sentences
// no constant of the module holds, written at the call, handed to refuse,
// put together from a constant and held by a constant of the standard
// library, and two constants named unlike a sentence, the first handed over
// twice.
func reported(cause error) []any {
	return []any{
		gst.NewErrorWithCause(500, "the sample broke", cause),
		refuse("the sample is refused"),
		module.NewError(403, lostMsg+" for good"),
		gst.NewError(405, http.MethodGet),
		i18n.Translate(lockedText),
		gst.NewError(423, lockedText),
		gst.NewError(503, shared.Quiet),
	}
}

// refuse hands its parameter to a constructor as it stands.
func refuse(msg string) *gst.Error { return gst.NewError(400, msg) }

// accepted answers with what the check reports nothing for: constants named
// as a sentence is, handed to a constructor, to refuse and to the variable
// forwarding a constructor, the last a constant of another package; messages
// put together as the request is served; and the result of a function.
func accepted(name string, cause error) []any {
	return []any{
		gst.NewError(404, lostMsg),
		refuse(foundMsg),
		module.NewError(503, shared.DownMsg),
		gst.NewError(400, fmt.Sprintf("the sample %s is odd", name)),
		gst.NewErrorWithCause(500, cause.Error(), cause),
		gst.NewError(404, returned()),
	}
}

// returned returns a sentence for its caller to answer with.
func returned() string { return returnedText }
