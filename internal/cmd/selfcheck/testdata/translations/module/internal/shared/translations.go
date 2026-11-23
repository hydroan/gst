// Package shared declares sentences other packages answer with.
package shared

import "example.com/module/internal/i18n"

const (
	// DownMsg is translated here, where it is declared.
	DownMsg = "the sample is down"
	// QuietMsg is handed to a constructor by another package and translated
	// nowhere.
	QuietMsg = "the sample is quiet"
)

func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		DownMsg: "样本已停用",
	})
}
