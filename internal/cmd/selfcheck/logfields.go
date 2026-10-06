package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// The import paths the check reads log field writes by: zap, whose
// constructors build the fields, and zapcore, whose Field they return; and,
// relative to the module, the package declaring the keys more than one stream
// writes and the two packages allowed to nest keys under an entry.
const (
	zapPath      = "go.uber.org/zap"
	zapcorePath  = "go.uber.org/zap/zapcore"
	logfieldPath = "internal/logfield"
	utilPath     = "util"
)

// keyValueMethod matches the logging methods taking their fields as key-value
// pairs, Infow(msg, key, value, ...) and With(key, value, ...), which type
// each field by its value the way zap.Any does.
var keyValueMethod = regexp.MustCompile(`^(?:Debug|Info|Warn|Error|Fatal|Panic|DPanic)w$|^With$`)

// fieldClass is the JSON type a log store maps a field's key to.
type fieldClass string

const (
	classString fieldClass = "a string"
	classNumber fieldClass = "a number"
	classBool   fieldClass = "a boolean"
	classArray  fieldClass = "an array"
	classObject fieldClass = "an object"
	// classUnknown is the class of a value of interface type, decided at run
	// time by the value's own type.
	classUnknown fieldClass = ""
)

// constructorClasses maps zap's field constructors to the class of the field
// each builds. Any and Reflect are absent, typing the field by the value,
// which classOfType reads; Object, Dict, Namespace and Inline nest keys under
// the entry and are judged by the package calling them.
var constructorClasses = map[string]fieldClass{
	"String": classString, "Stringp": classString, "Stringer": classString, "ByteString": classString, "Binary": classString,
	"Error": classString, "NamedError": classString, "Time": classString, "Timep": classString, "Stack": classString, "StackSkip": classString,
	"Complex128": classString, "Complex128p": classString, "Complex64": classString, "Complex64p": classString,
	"Int": classNumber, "Intp": classNumber, "Int64": classNumber, "Int64p": classNumber, "Int32": classNumber, "Int32p": classNumber,
	"Int16": classNumber, "Int16p": classNumber, "Int8": classNumber, "Int8p": classNumber,
	"Uint": classNumber, "Uintp": classNumber, "Uint64": classNumber, "Uint64p": classNumber, "Uint32": classNumber, "Uint32p": classNumber,
	"Uint16": classNumber, "Uint16p": classNumber, "Uint8": classNumber, "Uint8p": classNumber, "Uintptr": classNumber, "Uintptrp": classNumber,
	"Float64": classNumber, "Float64p": classNumber, "Float32": classNumber, "Float32p": classNumber, "Duration": classNumber, "Durationp": classNumber,
	"Bool": classBool, "Boolp": classBool,
	"Array": classArray, "Bools": classArray, "ByteStrings": classArray, "Complex128s": classArray, "Complex64s": classArray, "Durations": classArray,
	"Float64s": classArray, "Float32s": classArray, "Ints": classArray, "Int64s": classArray, "Int32s": classArray, "Int16s": classArray, "Int8s": classArray,
	"Strings": classArray, "Times": classArray, "Uints": classArray, "Uint64s": classArray, "Uint32s": classArray, "Uint16s": classArray, "Uint8s": classArray,
	"Uintptrs": classArray, "Errors": classArray, "Stringers": classArray, "Objects": classArray, "ObjectValues": classArray,
}

// nestingConstructors are the constructors nesting keys of their own under
// the entry: a store maps each nested key too, so an open key set grows its
// mapping without bound.
var nestingConstructors = map[string]bool{"Object": true, "Dict": true, "Namespace": true, "Inline": true}

// fieldWrite is one place the tree writes a log field: the key, the class the
// write gives it, how it is written and where.
type fieldWrite struct {
	key   string
	class fieldClass
	how   string
	file  string
	line  int
}

// checkLogFields reports the log field writes of the non-test files that
// break the one rule a log store mapping keys by type imposes, that a key has
// one type: a key internal/logfield declares written anywhere but through
// its constructor, a key written as two classes across the tree, and a
// key-value pair whose value is of interface type, typed at run time by the
// value. A write is a zap constructor call, whose name fixes the class, or a
// key-value pair of a sugared method (see keyValueMethod), whose value's
// static type fixes it the way zap.Any reads the value (see classOfType). A
// key that is not a constant is skipped: golangci-lint's loggercheck holds
// the pairs to their shape, and no linter compares the type a key is given
// across the tree, which is why this check exists. The module sources under
// internal/model and internal/service are copied into projects, which cannot
// import internal/logfield, so they are held to the type rule alone, as a
// project is. zap.Object, zap.Dict,
// zap.Namespace and zap.Inline nest keys under the entry and are reported
// outside internal/logfield and util, whose key sets are fixed; a zap
// constructor constructorClasses does not know is reported so the table stays
// complete.
func checkLogFields(root string, pkgs []*packages.Package) ([]violation, error) {
	checked := checkedPackages(pkgs)
	module := ""
	for _, p := range checked {
		if p.Module != nil {
			module = p.Module.Path
			break
		}
	}
	logfieldPkg := module + "/" + logfieldPath
	nesting := map[string]bool{logfieldPkg: true, module + "/" + utilPath: true}
	owned := ownedKeys(checked, logfieldPkg)

	type found struct {
		file, message string
		line          int
	}
	var all []found
	var writes []fieldWrite
	for _, p := range checked {
		if p.TypesInfo == nil {
			continue
		}
		for i, file := range p.Syntax {
			path := relative(root, p.CompiledGoFiles[i])
			if filepath.IsAbs(path) || strings.HasSuffix(path, "_test.go") {
				continue
			}
			report := func(pos ast.Node, format string, args ...any) {
				line := p.Fset.Position(pos.Pos()).Line
				all = append(all, found{file: path, line: line, message: fmt.Sprintf("Log field '%s:%d': ", path, line) + fmt.Sprintf(format, args...)})
			}
			record := func(pos ast.Node, key string, class fieldClass, how string, valueType types.Type) {
				if constructor, ok := owned[key]; ok && p.PkgPath != logfieldPkg && !isModuleSource(module, p.PkgPath) {
					report(pos, "the key %q is internal/logfield's; write it through logfield.%s, not %s", key, constructor, how)
					return
				}
				if class == classUnknown {
					report(pos, "the value of %q is of interface type %s, so the type of the field is decided at run time; render it with a typed constructor, zap.String with fmt.Sprint for a value of any type", key, types.TypeString(valueType, types.RelativeTo(p.Types)))
					return
				}
				writes = append(writes, fieldWrite{key: key, class: class, how: how, file: path, line: p.Fset.Position(pos.Pos()).Line})
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if fn := zapConstructor(p.TypesInfo, sel); fn != nil {
					name := fn.Name()
					switch {
					case name == "Skip":
					case nestingConstructors[name]:
						if !nesting[p.PkgPath] {
							report(call, "zap.%s nests keys under the entry; internal/logfield and util alone, whose key sets are fixed, may nest", name)
						}
					case name == "Any" || name == "Reflect":
						if key, ok := constString(p.TypesInfo, call.Args[0]); ok {
							t := p.TypesInfo.TypeOf(call.Args[1])
							record(call, key, classOfType(t, name == "Any"), "zap."+name, t)
						}
					default:
						class, known := constructorClasses[name]
						if !known {
							if takesKey(fn) {
								report(call, "zap.%s is a field constructor this check does not classify; add it to logfields.go", name)
							}
							return true
						}
						if key, ok := constString(p.TypesInfo, call.Args[0]); ok {
							record(call, key, class, "zap."+name, nil)
						}
					}
					return true
				}
				if selection, ok := p.TypesInfo.Selections[sel]; ok && selection.Kind() == types.MethodVal && keyValueMethod.MatchString(sel.Sel.Name) && takesKeyValues(selection.Type()) {
					start := 1
					if sel.Sel.Name == "With" {
						start = 0
					}
					for i := start; i < len(call.Args); {
						arg := call.Args[i]
						if isZapField(p.TypesInfo.TypeOf(arg)) {
							i++
							continue
						}
						key, ok := constString(p.TypesInfo, arg)
						if !ok || i+1 >= len(call.Args) {
							break
						}
						if t := p.TypesInfo.TypeOf(call.Args[i+1]); !isUntypedNil(t) {
							record(arg, key, classOfType(t, true), "a key-value pair of "+sel.Sel.Name, t)
						}
						i += 2
					}
				}
				return true
			})
		}
	}

	byKey := make(map[string][]fieldWrite)
	for _, w := range writes {
		byKey[w.key] = append(byKey[w.key], w)
	}
	for key, sites := range byKey {
		sort.Slice(sites, func(i, j int) bool {
			if sites[i].file != sites[j].file {
				return sites[i].file < sites[j].file
			}
			return sites[i].line < sites[j].line
		})
		classes := make(map[fieldClass]bool)
		parts := make([]string, 0, len(sites))
		for _, s := range sites {
			classes[s.class] = true
			parts = append(parts, fmt.Sprintf("%s at %s:%d", s.class, s.file, s.line))
		}
		if len(classes) > 1 {
			all = append(all, found{file: sites[0].file, line: sites[0].line, message: fmt.Sprintf("Log field %q: %s; a key has one type, a store mapping keys by type drops the entries of every other", key, strings.Join(parts, ", "))})
		}
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].file != all[j].file {
			return all[i].file < all[j].file
		}
		if all[i].line != all[j].line {
			return all[i].line < all[j].line
		}
		return all[i].message < all[j].message
	})
	violations := make([]violation, 0, len(all))
	for _, f := range all {
		violations = append(violations, violation{File: f.file, Message: f.message})
	}
	return violations, nil
}

// moduleSourceDirs are the directories, relative to the module, holding the
// sources of the framework's modules, which gg module copy copies into a
// project.
var moduleSourceDirs = []string{"internal/model/", "internal/service/"}

// isModuleSource reports whether the package at pkgPath is a module source,
// one of moduleSourceDirs of module.
func isModuleSource(module, pkgPath string) bool {
	for _, dir := range moduleSourceDirs {
		if strings.HasPrefix(pkgPath, module+"/"+dir) {
			return true
		}
	}
	return false
}

// ownedKeys returns the keys the constructors of the package at logfieldPkg
// write, each with the name of its constructor: the keys every other package
// writes through that constructor alone.
func ownedKeys(pkgs []*packages.Package, logfieldPkg string) map[string]string {
	owned := make(map[string]string)
	for _, p := range pkgs {
		if p.PkgPath != logfieldPkg || p.TypesInfo == nil {
			continue
		}
		for _, file := range p.Syntax {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv != nil || !ast.IsExported(fn.Name.Name) || fn.Body == nil {
					continue
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok && zapConstructor(p.TypesInfo, sel) != nil && len(call.Args) > 0 {
						if key, ok := constString(p.TypesInfo, call.Args[0]); ok {
							owned[key] = fn.Name.Name
						}
					}
					return true
				})
			}
		}
	}
	return owned
}

// zapConstructor returns the package-level function of zap that sel names,
// zap.String or zap.Object, and nil for any other selector.
func zapConstructor(info *types.Info, sel *ast.SelectorExpr) *types.Func {
	fn, ok := info.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != zapPath {
		return nil
	}
	if sig, ok := fn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return nil
	}
	return fn
}

// takesKey reports whether fn builds a field from a key: its first parameter
// is a string and it returns a zapcore.Field.
func takesKey(fn *types.Func) bool {
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Params().Len() == 0 || sig.Results().Len() != 1 || !isZapField(sig.Results().At(0).Type()) {
		return false
	}
	first, ok := types.Unalias(sig.Params().At(0).Type()).(*types.Basic)
	return ok && first.Kind() == types.String
}

// takesKeyValues reports whether t is the signature of a method taking its
// fields as key-value pairs: variadic, its last parameter a slice of the
// empty interface.
func takesKeyValues(t types.Type) bool {
	sig, ok := t.(*types.Signature)
	if !ok || !sig.Variadic() {
		return false
	}
	slice, ok := types.Unalias(sig.Params().At(sig.Params().Len() - 1).Type()).(*types.Slice)
	if !ok {
		return false
	}
	iface, ok := types.Unalias(slice.Elem()).(*types.Interface)
	return ok && iface.Empty()
}

// constString returns the value of e when e is a constant string, a literal
// or a named constant.
func constString(info *types.Info, e ast.Expr) (string, bool) {
	tv, ok := info.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

// isZapField reports whether t is zapcore.Field, the type every constructor
// returns and a key-value method takes as it is.
func isZapField(t types.Type) bool {
	named, ok := derefNamed(t)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == zapcorePath && named.Obj().Name() == "Field"
}

// isUntypedNil reports whether t is the type of the nil literal, whose field
// is JSON null and carries no type for a store to map.
func isUntypedNil(t types.Type) bool {
	basic, ok := types.Unalias(t).(*types.Basic)
	return ok && basic.Kind() == types.UntypedNil
}

var universeError = types.Universe.Lookup("error").Type()

// classOfType returns the class of the field a value of type t makes, read
// the way zap.Any reads the value when viaAny is true and the way zap.Reflect
// reads it otherwise: zap.Any gives a marshaler its own shape, a Stringer and
// an error a string, the basic types and the slices of them their own kinds,
// a time.Duration a number of nanoseconds, a time.Time a string, and hands
// everything else to reflection; zap.Reflect hands everything to reflection.
// A reflected struct, map, array or slice is an object or an array the
// encoder collapses into one string (see logger.newLogEncoder), a reflected
// scalar keeps its kind. A value of interface type, error aside, is typed at
// run time, classUnknown.
func classOfType(t types.Type, viaAny bool) fieldClass {
	t = types.Unalias(t)
	if viaAny {
		if hasMethod(t, "MarshalLogObject") {
			return classObject
		}
		if hasMethod(t, "MarshalLogArray") {
			return classArray
		}
	}
	if types.Identical(t, universeError) {
		return classString
	}
	switch u := t.(type) {
	case *types.Basic:
		info := u.Info()
		switch {
		case info&types.IsBoolean != 0:
			return classBool
		case info&types.IsString != 0, info&types.IsComplex != 0:
			return classString
		case info&types.IsNumeric != 0:
			return classNumber
		}
		return classUnknown
	case *types.Pointer:
		return classOfType(u.Elem(), viaAny)
	case *types.Named:
		if obj := u.Obj(); obj.Pkg() != nil && obj.Pkg().Path() == "time" {
			switch obj.Name() {
			case "Duration":
				return classNumber
			case "Time":
				return classString
			}
		}
		if viaAny && hasStringMethod(t) {
			return classString
		}
		if _, isInterface := u.Underlying().(*types.Interface); isInterface {
			return classUnknown
		}
		if slice, ok := u.Underlying().(*types.Slice); ok {
			// zap.Any matches the slices of the basic types by their exact
			// type, so a named slice reaches reflection whatever it holds.
			return classOfSlice(slice, false)
		}
		return classOfType(u.Underlying(), false)
	case *types.Interface:
		return classUnknown
	case *types.Slice:
		return classOfSlice(u, viaAny)
	case *types.Array, *types.Map, *types.Struct, *types.Signature, *types.Chan:
		return classString
	}
	return classUnknown
}

// classOfSlice returns the class of the field a slice of type u makes: read
// by zap.Any, a slice of a basic type is an array, a slice of bytes a string,
// and a slice of time.Duration, time.Time or error an array; any other slice,
// and every slice read by zap.Reflect, is reflected into an array the encoder
// collapses into one string.
func classOfSlice(u *types.Slice, viaAny bool) fieldClass {
	if !viaAny {
		return classString
	}
	switch elem := types.Unalias(u.Elem()).(type) {
	case *types.Basic:
		if elem.Kind() == types.Uint8 {
			return classString
		}
		return classArray
	case *types.Named:
		if obj := elem.Obj(); obj.Pkg() != nil && obj.Pkg().Path() == "time" && (obj.Name() == "Duration" || obj.Name() == "Time") {
			return classArray
		}
		if types.Identical(elem, universeError) {
			return classArray
		}
	}
	return classString
}

// hasMethod reports whether a value of type t, as it is, has a method named
// name.
func hasMethod(t types.Type, name string) bool {
	obj, _, _ := types.LookupFieldOrMethod(t, false, nil, name)
	_, ok := obj.(*types.Func)
	return ok
}

// hasStringMethod reports whether a value of type t is a fmt.Stringer: it has
// a String method taking nothing and returning one string.
func hasStringMethod(t types.Type) bool {
	obj, _, _ := types.LookupFieldOrMethod(t, false, nil, "String")
	fn, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Params().Len() != 0 || sig.Results().Len() != 1 {
		return false
	}
	result, ok := types.Unalias(sig.Results().At(0).Type()).(*types.Basic)
	return ok && result.Kind() == types.String
}
