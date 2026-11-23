package answer

import (
	"example.com/module/internal/i18n"
	"example.com/module/internal/shared"
)

const (
	// foundMsg is translated under its own name.
	foundMsg = "the sample is found"
	// takenMsg is answered with by its name and by the string written out.
	takenMsg = "the sample is taken"
	// refusedMsg reaches a constructor through refuse.
	refusedMsg = "the sample is refused"
	// renamedMsg renames a sentence of shared, which translates it.
	renamedMsg = shared.DownMsg
	// returnedMsg reaches a client through the result of a function.
	returnedMsg = "the sample is returned"
	// goneMsg is used by nothing but the registration.
	goneMsg = "the sample is gone"
)

// table holds translations where the check does not read them.
var table = map[string]string{"the sample is hidden": "样本已隐藏"}

func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		foundMsg:              "样本已找到",
		takenMsg:              "样本已被占用",
		refusedMsg:            "样本被拒绝",
		returnedMsg:           "样本已归还",
		goneMsg:               "样本不在了",
		strayMsg:              "样本走失了",
		"the sample is early": "样本来早了",
		shared.QuietMsg:       "样本很安静",
	})
	i18n.Register(i18n.Chinese, table)
}
