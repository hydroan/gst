// Package i18n holds the language a process answers its clients in and the
// translations of the sentences it answers them with.
//
// A sentence a client is answered with, the message of a refusal, is
// written in English. A package registers the translations of its own
// sentences from an init function (see Register), the process chooses its
// language once, as it boots (see SetLanguage), and a transport translates a
// sentence at the moment it answers with it (see Translate): the error
// carrying the sentence, and the log recording it, keep it as it is written.
// A sentence no translation is registered for is answered as it is written,
// which is how a project wording its own refusals in the language of its
// users has them answered.
//
// The description of a field a request was refused on is worded rather than
// translated: the validation writes it in the language of the process as it
// refuses the field, so the error and the log carry it in that language.
package i18n

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

// The languages the framework speaks, each by the BCP 47 tag a translation
// is registered under: English, the language every sentence is written in,
// and the languages a sentence is translated into.
const (
	English = "en"
	// Chinese is Chinese in the simplified script.
	Chinese = "zh"
	// TraditionalChinese is Chinese in the traditional script, worded the way
	// Taiwan writes it.
	TraditionalChinese = "zh-Hant"
)

// languages lists the languages the framework speaks, the one the sentences
// are written in first.
var languages = []string{English, Chinese, TraditionalChinese}

// dictionary is a language with the translations registered into it, none
// for English.
type dictionary struct {
	language     string
	translations map[string]string
}

var (
	// mu orders the writes of registered and inUse: a Register against
	// another and against a SetLanguage.
	mu sync.Mutex
	// registered holds the translations registered so far, by language and
	// then by sentence. The map of a language is replaced by a Register and
	// never written once it is there, so the one inUse hands to Translate is
	// read without the lock.
	registered = make(map[string]map[string]string)
	// inUse is the dictionary of the language SetLanguage chose; nil,
	// English, until it is called.
	inUse atomic.Pointer[dictionary]
)

// Register registers translations: sentences a package answers a client
// with, each with its wording in language, one of the languages the
// framework speaks beside English. A package calls it from an init function
// with the sentences it declares, so that every translation is there before
// the process answers anything.
//
// It panics on what only a mistake of the calling package brings about: a
// language sentences are not translated into, English, the one they are
// written in, among them; an empty translation; and a sentence registered
// with another translation already, since which of the two a client would be
// answered with would turn on the order packages initialize in.
func Register(language string, translations map[string]string) {
	if language == English || !slices.Contains(languages, language) {
		panic(fmt.Sprintf("i18n: Register of translations into %q; a sentence is translated into one of: %s", language, strings.Join(languages[1:], ", ")))
	}
	mu.Lock()
	defer mu.Unlock()
	merged := maps.Clone(registered[language])
	if merged == nil {
		merged = make(map[string]string, len(translations))
	}
	for sentence, translation := range translations {
		if translation == "" {
			panic(fmt.Sprintf("i18n: Register of %q into %s with an empty translation", sentence, language))
		}
		if present, ok := merged[sentence]; ok && present != translation {
			panic(fmt.Sprintf("i18n: Register of %q into %s as %q; it is registered as %q", sentence, language, translation, present))
		}
		merged[sentence] = translation
	}
	registered[language] = merged
	if current := inUse.Load(); current != nil && current.language == language {
		inUse.Store(&dictionary{language: language, translations: merged})
	}
}

// Translate returns sentence in the language the process answers in (see
// SetLanguage): the translation registered for it, and sentence itself where
// the process answers in English or no translation of it is registered.
func Translate(sentence string) string {
	current := inUse.Load()
	if current == nil {
		return sentence
	}
	if translation, ok := current.translations[sentence]; ok {
		return translation
	}
	return sentence
}
