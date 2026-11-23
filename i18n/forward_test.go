package i18n_test

import (
	"testing"

	"github.com/hydroan/gst/i18n"
	internali18n "github.com/hydroan/gst/internal/i18n"
	"github.com/stretchr/testify/require"
)

// TestRegisterForwardsToTheTranslationsOfTheProcess pins the public package
// as a forwarder: a translation a module registers through it is the one a
// process answering in that language answers with, and the language it names
// is the framework's own, as are the languages the package names.
func TestRegisterForwardsToTheTranslationsOfTheProcess(t *testing.T) {
	require.Equal(t, internali18n.Chinese, i18n.Chinese)
	require.Equal(t, internali18n.TraditionalChinese, i18n.TraditionalChinese)

	i18n.Register(i18n.Chinese, map[string]string{"the sample is forwarded": "样本已转交"})

	require.NoError(t, internali18n.SetLanguage(i18n.Chinese))
	t.Cleanup(func() { require.NoError(t, internali18n.SetLanguage(internali18n.English)) })
	require.Equal(t, "样本已转交", internali18n.Translate("the sample is forwarded"))
}
