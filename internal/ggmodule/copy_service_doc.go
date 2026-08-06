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
	methods   map[string][]string
}

type declDocInsert struct {
	kind token.Token
	name string
	doc  []string
}

func newSourceDocInserts() sourceDocInserts {
	return sourceDocInserts{
		functions: make(map[string][]string),
		methods:   make(map[string][]string),
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

func appendSourceComments(targetFile *ast.File, sourceComments ast.CommentMap, node ast.Node) {
	if targetFile == nil || sourceComments == nil || node == nil {
		return
	}
	targetFile.Comments = append(targetFile.Comments, sourceComments.Filter(node).Comments()...)
}

func insertStructDoc(code string, typeName string, docLines []string) string {
	if len(docLines) == 0 {
		return code
	}
	lines := strings.Split(code, "\n")
	typePrefix := "type " + typeName + " struct"
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), typePrefix) {
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

func insertMethodDoc(code string, receiverType string, methodName string, docLines []string) string {
	if len(docLines) == 0 {
		return code
	}
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "func (") || !strings.Contains(trimmed, " "+methodName+"(") {
			continue
		}
		if !strings.Contains(trimmed, "*"+receiverType+")") && !strings.Contains(trimmed, " "+receiverType+")") {
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

func insertFunctionDoc(code string, functionName string, docLines []string) string {
	if len(docLines) == 0 {
		return code
	}
	lines := strings.Split(code, "\n")
	funcPrefix := "func " + functionName + "("
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), funcPrefix) {
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

func insertDeclDoc(code string, insertDoc declDocInsert, serviceStruct string) string {
	if len(insertDoc.doc) == 0 || len(insertDoc.name) == 0 || insertDoc.name == serviceStruct {
		return code
	}
	lines := strings.Split(code, "\n")
	prefix := strings.ToLower(insertDoc.kind.String()) + " " + insertDoc.name
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), prefix) {
			continue
		}
		if i > 0 && strings.TrimSpace(lines[i-1]) == insertDoc.doc[len(insertDoc.doc)-1] {
			return code
		}
		doc := append([]string{}, insertDoc.doc...)
		lines = append(lines[:i], append(doc, lines[i:]...)...)
		return strings.Join(lines, "\n")
	}
	return code
}
