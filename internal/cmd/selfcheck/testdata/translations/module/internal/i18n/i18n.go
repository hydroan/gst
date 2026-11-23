// Package i18n stands for the package registering the translations and
// translating.
package i18n

// The languages a translation is registered under.
const (
	English = "en"
	Chinese = "zh"
)

// languages lists the languages the fixture speaks, the one its sentences
// are written in first.
var languages = []string{English, Chinese}

// Register stands for the registration of translations into language.
func Register(language string, translations map[string]string) {}

// Translate stands for the translation of sentence.
func Translate(sentence string) string { return sentence }
