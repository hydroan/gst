// Package datatypes holds the types a model declares the fields of a kind of
// its own with: a calendar day, a time of day, and a JSON document stored in
// one column, an array, an object or a value of any Go type. Each is the
// framework's own type, so what it writes in a request and a response, and
// what it reads from one, is the framework's contract, written here. The
// column it creates, and what it writes to the database and reads from it,
// are those of the gorm.io/datatypes type it stores through, so that what
// the library learns of a database reaches it with the library. The public
// datatypes package forwards the types to projects.
//
// Each type is declared in a file of its own, named after it, date.go for
// Date, with its constructors, its parser and its methods; its tests are in
// the test file of the same name. library.go declares no type: it tells
// which names of the library the package has its own for.
package datatypes
