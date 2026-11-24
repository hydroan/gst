// Package answer answers with sentences, translated or not.
package answer

import (
	"fmt"

	"example.com/module"
	"example.com/module/internal/i18n"
	"example.com/module/internal/shared"
	"example.com/module/internal/gst"
)

const (
	// lostMsg is handed to a constructor and translated nowhere.
	lostMsg = "the sample is lost"
	// wrappedMsg reaches a constructor through refuse and is translated
	// nowhere.
	wrappedMsg = "the sample is wrapped"
	// strayMsg is declared here, and translations.go registers a translation
	// under it.
	strayMsg = "the sample is astray"
	// lateMsg is translated by the registration of this file.
	lateMsg = "the sample is late"
)

// reported answers with the sentences the check reports for having no
// translation, one a line, the last for the package declaring it.
func reported(cause error) []any {
	return []any{
		gst.NewError(404, lostMsg),
		gst.NewErrorWithCause(500, "the sample broke", cause),
		refuse(wrappedMsg),
		module.NewError(403, "the sample is locked"),
		i18n.Translate("the sample is asleep"),
		gst.NewError(503, shared.QuietMsg),
	}
}

// refuse hands its parameter to a constructor as it stands.
func refuse(msg string) *gst.Error { return gst.NewError(400, msg) }

// accepted answers with what the check reports no sentence for: sentences
// translated under their constant, handed over by it, written out at the
// call or passed through refuse, sentences the package declaring them
// translates, handed over by its constant or by one renaming it, messages
// put together as the request is served, the result of a function, and the
// two sentences whose translations are reported for where they are
// registered.
func accepted(name string, cause error) []any {
	return []any{
		gst.NewError(409, foundMsg),
		gst.NewError(409, takenMsg),
		gst.NewError(409, "the sample is taken"),
		refuse(refusedMsg),
		gst.NewError(503, shared.DownMsg),
		gst.NewError(503, renamedMsg),
		gst.NewError(400, fmt.Sprintf("the sample %s is odd", name)),
		gst.NewErrorWithCause(500, cause.Error(), cause),
		gst.NewError(404, returned()),
		gst.NewError(410, strayMsg),
		gst.NewError(425, lateMsg),
	}
}

// returned returns a sentence for its caller to answer with.
func returned() string { return returnedMsg }

func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		lateMsg: "样本迟到了",
	})
}
