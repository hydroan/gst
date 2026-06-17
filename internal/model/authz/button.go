package modelauthz

import "github.com/hydroan/gst/model"

type Button struct {
	Name string // button display name
	Code string // unique button code/identifier
	Icon string // icon name from backend

	Status   int   // 0: disabled, 1: enabled
	Visiable *bool // whether the button is rendered in UI

	MenuID string // parent menu id

	model.Base
}

func (Button) Purge() bool { return true }
