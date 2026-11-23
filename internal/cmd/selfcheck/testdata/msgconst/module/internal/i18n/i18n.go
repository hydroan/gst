// Package i18n stands for the package registering the translations and
// translating.
package i18n

// Chinese is a language a translation is registered under.
const Chinese = "zh"

// Register stands for the registration of translations into language.
func Register(language string, translations map[string]string) {}

// Translate stands for the translation of sentence.
func Translate(sentence string) string { return sentence }
