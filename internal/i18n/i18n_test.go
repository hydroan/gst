package i18n_test

import (
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/i18n"
	"github.com/stretchr/testify/require"
)

func TestTranslateAnswersInTheLanguageInUse(t *testing.T) {
	i18n.Register(i18n.Chinese, map[string]string{"the sample is busy": "样本正忙"})

	require.Equal(t, i18n.English, i18n.Language())
	require.Equal(t, "the sample is busy", i18n.Translate("the sample is busy"), "English until a language is set")

	answerIn(t, i18n.Chinese)
	require.Equal(t, i18n.Chinese, i18n.Language())
	require.Equal(t, "样本正忙", i18n.Translate("the sample is busy"))
	require.Equal(t, "the sample is idle", i18n.Translate("the sample is idle"), "a sentence no translation is registered for")
	require.Equal(t, "样本不存在", i18n.Translate("样本不存在"), "a sentence written in the language of its readers")

	require.NoError(t, i18n.SetLanguage(i18n.English))
	require.Equal(t, i18n.English, i18n.Language())
	require.Equal(t, "the sample is busy", i18n.Translate("the sample is busy"))
}

func TestRegisterAddsToTheTranslationsOfALanguage(t *testing.T) {
	t.Run("a translation registered into the language in use is answered at once", func(t *testing.T) {
		answerIn(t, i18n.Chinese)
		require.Equal(t, "the sample is late", i18n.Translate("the sample is late"))
		i18n.Register(i18n.Chinese, map[string]string{"the sample is late": "样本已迟到"})
		require.Equal(t, "样本已迟到", i18n.Translate("the sample is late"))
	})

	t.Run("packages registering one sentence with one translation agree", func(t *testing.T) {
		i18n.Register(i18n.Chinese, map[string]string{"the sample is shared": "样本已共享"})
		require.NotPanics(t, func() {
			i18n.Register(i18n.Chinese, map[string]string{"the sample is shared": "样本已共享"})
		})
	})

	t.Run("a sentence registered with another translation", func(t *testing.T) {
		i18n.Register(i18n.Chinese, map[string]string{"the sample is taken": "样本已被占用"})
		require.PanicsWithValue(t, `i18n: Register of "the sample is taken" into zh as "样本被占用"; it is registered as "样本已被占用"`, func() {
			i18n.Register(i18n.Chinese, map[string]string{"the sample is taken": "样本被占用"})
		})
		answerIn(t, i18n.Chinese)
		require.Equal(t, "样本已被占用", i18n.Translate("the sample is taken"))
	})

	t.Run("an empty translation", func(t *testing.T) {
		require.PanicsWithValue(t, `i18n: Register of "the sample is blank" into zh with an empty translation`, func() {
			i18n.Register(i18n.Chinese, map[string]string{"the sample is blank": ""})
		})
	})

	t.Run("the language the sentences are written in", func(t *testing.T) {
		require.PanicsWithValue(t, `i18n: Register of translations into "en"; a sentence is translated into one of: `+translatedInto(), func() {
			i18n.Register(i18n.English, map[string]string{"the sample is plain": "the sample is plain"})
		})
	})

	t.Run("a language the framework does not speak", func(t *testing.T) {
		require.PanicsWithValue(t, `i18n: Register of translations into "fr"; a sentence is translated into one of: `+translatedInto(), func() {
			i18n.Register("fr", map[string]string{"the sample is new": "l'échantillon est nouveau"})
		})
	})
}

func TestSetLanguageChoosesTheLanguageATagNames(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, i18n.SetLanguage(i18n.English)) })

	for _, tc := range []struct {
		tag  string
		want string
	}{
		{tag: "zh", want: i18n.Chinese},
		{tag: "zh-CN", want: i18n.Chinese},
		{tag: "zh_CN", want: i18n.Chinese},
		{tag: "ZH-cn", want: i18n.Chinese},
		{tag: "zh-Hans", want: i18n.Chinese},
		{tag: "zh_Hans", want: i18n.Chinese},
		{tag: "zh-Hans-CN", want: i18n.Chinese},
		{tag: "zh-SG", want: i18n.Chinese},
		{tag: "zh-TW", want: i18n.TraditionalChinese},
		{tag: "zh_TW", want: i18n.TraditionalChinese},
		{tag: "zh-Hant", want: i18n.TraditionalChinese},
		{tag: "zh_Hant", want: i18n.TraditionalChinese},
		{tag: "zh-Hant-TW", want: i18n.TraditionalChinese},
		{tag: "zh-HK", want: i18n.TraditionalChinese},
		{tag: "zh-MO", want: i18n.TraditionalChinese},
		{tag: "en", want: i18n.English},
		{tag: "EN", want: i18n.English},
		{tag: "en-US", want: i18n.English},
		{tag: "en_GB", want: i18n.English},
	} {
		t.Run(tc.tag, func(t *testing.T) {
			require.NoError(t, i18n.SetLanguage(tc.tag))
			require.Equal(t, tc.want, i18n.Language())
		})
	}
}

func TestSetLanguageRefusesATagNamingNoLanguageSpoken(t *testing.T) {
	answerIn(t, i18n.Chinese)

	for _, tc := range []struct {
		name string
		tag  string
		want string
	}{
		{name: "another language", tag: "ja", want: `"ja" is no language the framework speaks: it speaks ` + spoken()},
		{name: "a language written like one spoken", tag: "yue", want: `"yue" is no language the framework speaks: it speaks ` + spoken()},
		{name: "no language determined", tag: "und", want: `"und" is no language the framework speaks: it speaks ` + spoken()},
		{name: "no tag", tag: "", want: `"" is not a language tag: language: tag is not well-formed`},
		{name: "the name of a language", tag: "中文", want: `"中文" is not a language tag: language: tag is not well-formed`},
		{name: "a subtag no language carries", tag: "xx", want: `"xx" is not a language tag: language: subtag "xx" is well-formed but unknown`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.EqualError(t, i18n.SetLanguage(tc.tag), tc.want)
			require.Equal(t, i18n.Chinese, i18n.Language(), "the language in use stays")
		})
	}
}

func TestLanguagesListsEnglishFirst(t *testing.T) {
	require.Equal(t, []string{i18n.English, i18n.Chinese, i18n.TraditionalChinese}, i18n.Languages())

	listed := i18n.Languages()
	listed[0] = "changed"
	require.Equal(t, i18n.English, i18n.Languages()[0], "the list handed out is the caller's own")
}

// answerIn sets the language the process answers in to the one tag names,
// for the rest of the test.
func answerIn(t *testing.T, tag string) {
	t.Helper()
	require.NoError(t, i18n.SetLanguage(tag))
	t.Cleanup(func() { require.NoError(t, i18n.SetLanguage(i18n.English)) })
}

// spoken lists the languages the framework speaks the way a refusal names
// them, and translatedInto the ones a sentence is translated into: the
// refusals are held to the list as it stands, which
// TestLanguagesListsEnglishFirst pins.
func spoken() string { return strings.Join(i18n.Languages(), ", ") }

func translatedInto() string { return strings.Join(i18n.Languages()[1:], ", ") }
