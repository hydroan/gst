// Package i18n registers the translations of the sentences a module, or a
// project, answers its clients with.
//
// A process answers its clients in the language its configuration names,
// app.language, English when it names none. The sentences of the framework
// are translated by the framework. The sentences of the code written on it,
// the messages of the errors a service or a model hook answers with, are
// written in English and translated by the package answering with them: it
// declares each as a constant it answers with, and registers under the
// constant, from an init function, the wording of the sentence in the
// languages the framework speaks beside English. A request is then answered
// with the translation, over HTTP and over gRPC alike.
//
//	const sampleSealedMsg = "the sample is sealed"
//
//	func init() {
//		i18n.Register(i18n.Chinese, map[string]string{
//			sampleSealedMsg: "样本已封存",
//		})
//		i18n.Register(i18n.TraditionalChinese, map[string]string{
//			sampleSealedMsg: "樣本已封存",
//		})
//	}
//
// A sentence no translation is registered for is answered as it is written,
// which is how a project wording its refusals in the language of its users
// has them answered, and what a message put together as a request is served
// is answered as.
//
// The implementation lives in internal/i18n; this package forwards the part
// a module uses.
package i18n

import "github.com/hydroan/gst/internal/i18n"

// The languages a sentence is translated into beside the English it is
// written in.
const (
	// Chinese names Chinese in the simplified script.
	Chinese = i18n.Chinese
	// TraditionalChinese names Chinese in the traditional script, worded the
	// way Taiwan writes it.
	TraditionalChinese = i18n.TraditionalChinese
)

// Register registers translations: sentences the calling package answers a
// client with, each with its wording in language. A package calls it from an
// init function, so that every translation is there before the process
// answers anything, and it panics on what only a mistake of the calling
// package brings about: a language sentences are not translated into, an
// empty translation, or a sentence registered with another translation
// already.
func Register(language string, translations map[string]string) {
	i18n.Register(language, translations)
}
