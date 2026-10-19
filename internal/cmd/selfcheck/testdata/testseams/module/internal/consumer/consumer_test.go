package consumer

import (
	"testing"

	"example.com/module/internal/cache/alt"
	"example.com/module/internal/companion"
	"example.com/module/internal/onlytests"
	"example.com/module/internal/seamed"
	"example.com/module/internal/support"
	"example.com/module/pkg"
)

func TestSum(t *testing.T) {
	support.Helper(t)
	if Sum()+onlytests.Exported()+alt.Cache()+pkg.Public()+seamed.Hook+companion.Companion()+companion.Knob+companion.NewHolder(1).N() != 18 {
		t.Fatal(Sum())
	}
}
