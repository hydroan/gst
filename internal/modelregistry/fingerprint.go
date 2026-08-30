package modelregistry

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/hydroan/gst/types"
)

// SchemaFingerprint returns a short hash of everything the registered models
// derive their tables from: their table names, the fields they declare with
// the tags gorm reads a column definition out of, and the indexes they
// declare. It is empty while nothing is registered.
//
// The hash answers one question — may a schema prepared for an earlier run be
// reused as it is — and is built to err towards saying no. It hashes
// declarations rather than rendered DDL, which keeps it independent of any
// particular rendering, and it hashes them whole, so a change that turns out
// never to reach the DDL costs a rebuild rather than a wrong answer.
//
// What it must not miss is a change no later migration can repair. A
// migration adds the columns a model gained and alters the ones whose
// definition moved, but it never drops what a model stopped declaring: a
// removed field or index is exactly the case that has to change the hash.
// Both are hashed by name, and so is the gorm version, which is the one thing
// that could map an unchanged declaration onto a differently named column.
func SchemaFingerprint() string {
	registeredMu.Lock()
	models := make([]types.Model, len(registeredModels))
	copy(models, registeredModels)
	registeredMu.Unlock()

	if len(models) == 0 {
		return ""
	}

	declarations := make([]string, 0, len(models))
	for _, m := range models {
		declarations = append(declarations, modelDeclaration(m))
	}
	// Registration order follows package initialization, which is no part of
	// the schema; sorting makes one set of models hash the same however it
	// happened to arrive.
	slices.Sort(declarations)

	sum := sha256.New()
	fmt.Fprintln(sum, gormVersion())
	for _, declaration := range declarations {
		fmt.Fprintln(sum, declaration)
	}
	return hex.EncodeToString(sum.Sum(nil)[:8])
}

// modelDeclaration renders everything m declares about its table.
func modelDeclaration(m types.Model) string {
	typ := reflect.TypeOf(m)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	var declaration strings.Builder
	fmt.Fprintf(&declaration, "%s table=%s", typ.String(), m.TableName())
	writeFieldDeclarations(&declaration, typ, map[reflect.Type]bool{})
	if declarer, ok := asIndexer(m); ok {
		for _, index := range declarer.Indexes() {
			fmt.Fprintf(&declaration, " index=%v unique=%t", index.Fields, index.Unique)
		}
	}
	return declaration.String()
}

// writeFieldDeclarations writes the fields typ declares, walking into embedded
// structs because a column promoted from one is a column all the same.
//
// The names written are Go field names and the tags are written verbatim: this
// records what a model declares, it does not resolve it into columns, which
// stays modelschema's job alone.
func writeFieldDeclarations(declaration *strings.Builder, typ reflect.Type, seen map[reflect.Type]bool) {
	if seen[typ] {
		return
	}
	seen[typ] = true

	for field := range typ.Fields() {
		fmt.Fprintf(declaration, " field=%s type=%s tag=%q", field.Name, field.Type.String(), string(field.Tag))

		embedded := field.Type
		for embedded.Kind() == reflect.Pointer {
			embedded = embedded.Elem()
		}
		if field.Anonymous && embedded.Kind() == reflect.Struct {
			writeFieldDeclarations(declaration, embedded, seen)
		}
	}
}

// gormVersion identifies the build that turns a declaration into DDL. A model
// can keep its declaration while a new gorm maps it onto a different column,
// which no migration would repair afterwards, so the version belongs in the
// hash. A build with no version information reads as unknown, which is stable
// and therefore no worse than leaving it out.
func gormVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "gorm=unknown"
	}
	for _, dep := range info.Deps {
		if dep.Path == "gorm.io/gorm" {
			return "gorm=" + dep.Version
		}
	}
	return "gorm=unknown"
}
