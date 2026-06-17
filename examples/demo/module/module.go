package module

import (
	"github.com/hydroan/gst/module/iam"
)

func init() {
	iam.Register(iam.Config{
		DefaultUsers: []*iam.User{
			{
				Username: "root",
				Password: "toor", // gitguardian:ignore
			},
		},
	})
}
