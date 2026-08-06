package ggmodule

import (
	"go/ast"
	"go/token"
	"sort"
	"strings"
)

// sourceDocInserts holds doc comments detached from the source declarations
// during the merge. mergeModuleServiceSource re-inserts them as text after the
// merged file has been printed.
type sourceDocInserts struct {
	decls     []declDocInsert
	functions map[string][]string
	methods   map[methodDocKey][]string
}

type declDocInsert struct {
	kind token.Token
	name string
	doc  []string
}

// methodDocKey identifies a method doc by the receiver type the method ends up
// on. Methods merged onto the target service struct and methods that keep their
// own source receiver can share a name, so the receiver has to be part of the
// key for both docs to survive.
type methodDocKey struct {
	receiver string
	name     string
}

func newSourceDocInserts() sourceDocInserts {
	return sourceDocInserts{
		functions: make(map[string][]string),
		methods:   make(map[methodDocKey][]string),
	}
}

func commentGroupLines(doc *ast.CommentGroup) []string {
	if doc == nil {
		return nil
	}
	lines := make([]string, 0, len(doc.List))
	for _, comment := range doc.List {
		lines = append(lines, comment.Text)
	}
	return lines
}

func retargetDocLines(docLines []string, sourceName string, targetName string) []string {
	if len(docLines) == 0 || sourceName == "" || targetName == "" || sourceName == targetName {
		return docLines
	}
	retargeted := append([]string{}, docLines...)
	sourcePrefix := "// " + sourceName
	if suffix, ok := strings.CutPrefix(retargeted[0], sourcePrefix); ok {
		retargeted[0] = "// " + targetName + suffix
	}
	return retargeted
}

func firstGenDeclSpecName(decl *ast.GenDecl) string {
	if decl == nil {
		return ""
	}
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			return s.Name.Name
		case *ast.ValueSpec:
			if len(s.Names) > 0 {
				return s.Names[0].Name
			}
		}
	}
	return ""
}

func sortedDocNames(docs map[string][]string) []string {
	names := make([]string, 0, len(docs))
	for name := range docs {
		if len(docs[name]) == 0 {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedMethodDocKeys(docs map[methodDocKey][]string) []methodDocKey {
	keys := make([]methodDocKey, 0, len(docs))
	for key := range docs {
		if len(docs[key]) == 0 {
			continue
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].receiver == keys[j].receiver {
			return keys[i].name < keys[j].name
		}
		return keys[i].receiver < keys[j].receiver
	})
	return keys
}

func appendSourceComments(targetFile *ast.File, sourceComments ast.CommentMap, node ast.Node) {
	if targetFile == nil || sourceComments == nil || node == nil {
		return
	}
	targetFile.Comments = append(targetFile.Comments, sourceComments.Filter(node).Comments()...)
}

// insertDocBefore puts docLines directly above the first printed line that
// match accepts. Doc comments are re-attached as text after the merged file has
// been printed, so the anchor is a line of output rather than an AST node.
func insertDocBefore(code string, docLines []string, match func(trimmed string) bool) string {
	if len(docLines) == 0 {
		return code
	}
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		if !match(strings.TrimSpace(line)) {
			continue
		}
		if i > 0 && strings.TrimSpace(lines[i-1]) == docLines[len(docLines)-1] {
			return code
		}
		insert := append([]string{}, docLines...)
		lines = append(lines[:i], append(insert, lines[i:]...)...)
		return strings.Join(lines, "\n")
	}
	return code
}

func insertStructDoc(code string, typeName string, docLines []string) string {
	typePrefix := "type " + typeName + " struct"
	return insertDocBefore(code, docLines, func(trimmed string) bool {
		return strings.HasPrefix(trimmed, typePrefix)
	})
}

func insertMethodDoc(code string, receiverType string, methodName string, docLines []string) string {
	return insertDocBefore(code, docLines, func(trimmed string) bool {
		if !strings.HasPrefix(trimmed, "func (") || !strings.Contains(trimmed, " "+methodName+"(") {
			return false
		}
		return strings.Contains(trimmed, "*"+receiverType+")") || strings.Contains(trimmed, " "+receiverType+")")
	})
}

func insertFunctionDoc(code string, functionName string, docLines []string) string {
	funcPrefix := "func " + functionName + "("
	return insertDocBefore(code, docLines, func(trimmed string) bool {
		return strings.HasPrefix(trimmed, funcPrefix)
	})
}

func insertDeclDoc(code string, insertDoc declDocInsert, serviceStruct string) string {
	if len(insertDoc.name) == 0 || insertDoc.name == serviceStruct {
		return code
	}
	prefix := strings.ToLower(insertDoc.kind.String()) + " " + insertDoc.name
	return insertDocBefore(code, insertDoc.doc, func(trimmed string) bool {
		return strings.HasPrefix(trimmed, prefix)
	})
}
