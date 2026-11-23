// Package i18n stands for the public package forwarding the registration to
// the module sources.
package i18n

import "example.com/module/internal/i18n"

// Chinese forwards the language.
const Chinese = i18n.Chinese

// Register forwards the registration.
func Register(language string, translations map[string]string) {
	i18n.Register(language, translations)
}
