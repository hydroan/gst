package module

import (
	"github.com/hydroan/gst/module/iam"
)

func init() {
	iam.Register(iam.Config{
		DefaultUsers: []*iam.DefaultUser{
			{
				Username: "root",
				Password: "toor", // gitguardian:ignore
			},
		},
	})
}
