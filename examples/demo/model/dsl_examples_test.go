package model_test

import (
	"testing"

	rootmodel "demo/model"
	"demo/model/archive"
	archivedocument "demo/model/archive/document"
	"demo/model/auth"
	"demo/model/record"
	"demo/model/tool"
)

type designer interface {
	Design()
}

func TestDemoDSLModelsAreAvailable(t *testing.T) {
	tests := []struct {
		name  string
		model designer
	}{
		{name: "record resource", model: rootmodel.Record{}},
		{name: "item resource", model: record.Item{}},
		{name: "entry utility action", model: tool.Entry{}},
		{name: "login public action", model: auth.Login{}},
		{name: "archive document resource", model: archive.Document{}},
		{name: "archive document seal action", model: archivedocument.Seal{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.model.Design()
		})
	}
}
