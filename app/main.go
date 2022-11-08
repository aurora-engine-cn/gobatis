package main

import (
	"fmt"
	"gitee.com/aurora-engine/sgo"
)

type Mapper func(any)

func main() {
	ctx := map[string]any{
		"arr":  []int{1, 2, 3, 4},
		"name": "saber",
	}
	sgo := sgo.NewSgo()
	sgo.LoadXml("/")
	sql, err := sgo.Sql("user.select03", ctx)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println(sql)
}
