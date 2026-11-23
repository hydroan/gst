package answer

import "example.com/module/internal/i18n"

const (
	// lostMsg and foundMsg are named as a sentence is.
	lostMsg  = "the sample is lost"
	foundMsg = "the sample is found"
	// lockedText is handed to two functions taking a sentence.
	lockedText = "the sample is locked"
	// returnedText reaches a client through the result of a function: it is
	// a sentence by the translation registered under it.
	returnedText = "the sample is returned"
)

func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		lostMsg:      "样本已丢失",
		foundMsg:     "样本已找到",
		lockedText:   "样本已锁定",
		returnedText: "样本已归还",
	})
}
