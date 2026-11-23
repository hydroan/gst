package datatypes

// LibraryPath is the import path of gorm.io/datatypes, the library the types
// of the package store through.
const LibraryPath = "gorm.io/datatypes"

// libraryNames maps the names of gorm.io/datatypes the package has its own
// for, the types a model declares and the functions making their values, to
// the name written in their place.
var libraryNames = map[string]string{
	"Date":         "Date",
	"Time":         "TimeOfDay",
	"NewTime":      "NewTimeOfDay",
	"JSON":         "JSON[json.RawMessage]",
	"JSONType":     "JSON",
	"NewJSONType":  "NewJSON",
	"JSONSlice":    "JSONSlice",
	"NewJSONSlice": "NewJSONSlice",
	"JSONMap":      "JSONMap",
}

// Replacement returns the name of the package written in place of name, a
// name gorm.io/datatypes exports: TimeOfDay for Time, NewJSON for
// NewJSONType, JSON[json.RawMessage] for the library's JSON; false for a
// name the package has nothing for, a JSON query or a UUID among them. It is
// the one account of what stands in for what: the refusal of a model's
// columns (modelschema.ValidateColumnTypes) and the Gorm datatypes ban of
// gg check both read it, and the tests of the package hold it to the names
// the library exports.
func Replacement(name string) (string, bool) {
	own, ok := libraryNames[name]
	return own, ok
}
