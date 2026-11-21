package sample

// RecordCreateReq declares one rule under validate, one under binding, and
// mentions the word in a tag that is neither.
type RecordCreateReq struct {
	Title  string `json:"title" validate:"required,max=64"`
	Remark string `json:"remark" binding:"max=255"`
	Note   string `json:"note" gorm:"comment:validate later"`
}

// ItemCreateReq declares rules under both tags on one field.
type ItemCreateReq struct {
	Code string `json:"code" binding:"required" validate:"len=6"`
}
