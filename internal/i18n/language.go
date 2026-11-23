package i18n

import (
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"golang.org/x/text/language"
)

// matcher matches a language tag to the closest of languages, by index.
var matcher = func() language.Matcher {
	tags := make([]language.Tag, 0, len(languages))
	for _, name := range languages {
		tags = append(tags, language.MustParse(name))
	}
	return language.NewMatcher(tags)
}()

// Languages returns the languages the framework speaks, English, the one
// every sentence is written in, first.
func Languages() []string {
	return slices.Clone(languages)
}

// Language returns the language the process answers in, one of Languages:
// English until SetLanguage chooses another.
func Language() string {
	if current := inUse.Load(); current != nil {
		return current.language
	}
	return English
}

// SetLanguage sets the language the process answers in to the one of
// Languages that tag names, a BCP 47 language tag the way a configuration
// spells it, with a region or a script or without, in either case and with
// a hyphen or an underscore: zh, zh-CN and zh_Hans name Chinese, zh-TW,
// zh-HK and zh_Hant name Chinese in the traditional script, en-US names
// English.
//
// A tag that is not well-formed is refused, and so is one naming a language
// the framework does not speak: a process answering its users in a language
// other than the one configured for them is told so as it boots.
func SetLanguage(tag string) error {
	asked, err := language.Parse(tag)
	if err != nil {
		return errors.Wrapf(err, "%q is not a language tag", tag)
	}
	// Only a match the matcher is sure of names the language: it answers
	// every tag with the closest it has, a guess for a tag none is close to.
	_, index, confidence := matcher.Match(asked)
	if confidence < language.High {
		return errors.Newf("%q is no language the framework speaks: it speaks %s", tag, strings.Join(languages, ", "))
	}
	spoken := languages[index]
	mu.Lock()
	defer mu.Unlock()
	inUse.Store(&dictionary{language: spoken, translations: registered[spoken]})
	return nil
}
