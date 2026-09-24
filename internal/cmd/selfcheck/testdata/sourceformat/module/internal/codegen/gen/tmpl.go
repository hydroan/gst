package gen

import "text/template"

// tmpl renders its output from a template.
var tmpl = template.Must(template.New("file").Parse("package sample\n"))
