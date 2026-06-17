package dsl

import "os"

var (
	input1 string
	input2 string
	input3 string
	input4 string
	input5 string
	input6 string
	input7 string
)

func init() {
	data1, err := os.ReadFile("./testdata/user.go")
	if err != nil {
		panic(err)
	}
	input1 = string(data1)

	data2, err := os.ReadFile("./testdata/user2.go")
	if err != nil {
		panic(err)
	}
	input2 = string(data2)

	data3, err := os.ReadFile("./testdata/user3_4.go")
	if err != nil {
		panic(err)
	}
	input3 = string(data3)

	data4, err := os.ReadFile("./testdata/user4.go")
	if err != nil {
		panic(err)
	}
	input4 = string(data4)

	data5, err := os.ReadFile("./testdata/user5.go")
	if err != nil {
		panic(err)
	}
	input5 = string(data5)

	data6, err := os.ReadFile("./testdata/user6_7.go")
	if err != nil {
		panic(err)
	}
	input6 = string(data6)

	data7, err := os.ReadFile("./testdata/user8_9.go")
	if err != nil {
		panic(err)
	}
	input7 = string(data7)
}
