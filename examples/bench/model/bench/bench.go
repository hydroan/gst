package bench

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Bench struct {
	Field1 string  `json:"field1"`
	Field2 int     `json:"field2"`
	Field3 *string `json:"field3"`
	Field4 *int    `json:"field4"`
	Field5 bool    `json:"field5"`

	DryRun bool `json:"dry_run" query:"dry_run" gorm:"-"`

	model.Base
	model.Query
}

func (Bench) TableName() string { return "benches" }
func (Bench) Purge() bool       { return true }

type (
	PingRsp struct {
		Msg string `json:"msg"`
	}

	GetRsp struct {
		Msg string `json:"msg"`
	}
	ListRsp struct {
		Msg string `json:"msg"`
	}

	CreateReq struct {
		Field1 string  `json:"field1"`
		Field2 int     `json:"field2"`
		Field3 *string `json:"field3"`
		Field4 *int    `json:"field4"`
	}
	CreateRsp = Bench

	UpdateReq struct {
		Field1 string  `json:"field1"`
		Field2 int     `json:"field2"`
		Field3 *string `json:"field3"`
		Field4 *int    `json:"field4"`
	}
	UpdateRsp struct {
		Msg string `json:"msg"`
	}

	DeleteRsp struct {
		Msg string `json:"msg"`
	}

	UpdateByIDReq struct {
		Field1 string `json:"field1"`
	}
	UpdateByIDRsp struct {
		Msg string `json:"msg"`
	}
)

func (Bench) Design() {
	Migrate()

	Route("bench/ping", func() {
		List(func() {
			Public()
			Service("ping")
			Result[*PingRsp]()
		})
	})

	Route("bench/get", func() {
		Get(func() {
			Public()
			Exact()
			Service("get")
			Result[*GetRsp]()
		})
	})

	Route("bench/list", func() {
		List(func() {
			Public()
			Service()
		})
	})
	Route("bench/list2", func() {
		List(func() {
			Public()
			Service("list2")
			Result[*ListRsp]()
		})
	})

	Route("bench/create", func() {
		Create(func() {
			Public()
			Service("create")
			Payload[*CreateReq]()
			Result[*CreateRsp]()
		})
	})

	Route("bench/update", func() {
		Update(func() {
			Public()
			Service("update")
			Payload[*UpdateReq]()
			Result[*UpdateRsp]()
		})
	})
	Route("bench/delete", func() {
		Delete(func() {
			Public()
			Service("delete")
			Result[*DeleteRsp]()
		})
	})
	Route("bench/updatebyid", func() {
		Patch(func() {
			Public()
			Service("updatebyid")
			Payload[*UpdateByIDReq]()
			Result[*UpdateByIDRsp]()
		})
	})
}
