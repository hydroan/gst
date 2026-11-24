package sample

import "example.com/module/i18n"

const (
	// unknownMsg is translated through the public package.
	unknownMsg = "the sample is unknown"
	// unpaidMsg is translated nowhere.
	unpaidMsg = "the sample is unpaid"
)

func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		unknownMsg: "样本未知",
	})
}
