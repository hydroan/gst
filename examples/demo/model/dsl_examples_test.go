package model_test

import (
	"testing"

	rootmodel "demo/model"
	"demo/model/auth"
	"demo/model/common"
	"demo/model/config"
	configfile "demo/model/config/file"
	"demo/model/record"
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
		{name: "search utility action", model: common.Search{}},
		{name: "login public action", model: auth.Login{}},
		{name: "config file resource", model: config.File{}},
		{name: "config file encrypt action", model: configfile.Encrypt{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.model.Design()
		})
	}
}
