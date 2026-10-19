package consumer

import (
	"example.com/module/internal/companion"
	"example.com/module/internal/onlytests"
	"example.com/module/internal/seamed"
	"example.com/module/internal/used"
)

// Sum uses what other packages export.
func Sum() int {
	return used.Used() + onlytests.Kept() + seamed.Leaked() + companion.Used() + companion.Holder{}.N()
}
