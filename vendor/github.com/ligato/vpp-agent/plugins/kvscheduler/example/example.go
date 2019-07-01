package main

import (
	"encoding/json"
	"fmt"

	"github.com/ligato/vpp-agent/plugins/kvscheduler/internal/utils"
)

type Int int

func (a Int) MarshalJSON() ([]byte, error) {
	test := a / 100
	return json.Marshal(fmt.Sprintf("%d-%d", a, test))
}

func (a Int) MarshalText() (text []byte, err error) {
	test := a / 10
	return []byte(fmt.Sprintf("%d-%d", a, test)), nil
}

type Something struct {
	Keys utils.KeySet
}

func main() {

	/*
	array := []Int{100, 200}
	arrayJson, err := json.Marshal(array)
	fmt.Println("array", string(arrayJson), err)

	maps := map[Int]bool{
		100: true,
		200: true,
	}
	mapsJson, err := json.Marshal(maps)
	fmt.Println("map wtf?", string(mapsJson), err)
	fmt.Println("map must be:", `{"100-10":true, "200-20":true}`)
	*/

	keySet := Something{
		Keys: utils.NewMapBasedKeySet("ahoj", "milan"),
	}
	keySetJson, err := json.Marshal(keySet)
	fmt.Println("JSON: ", string(keySetJson), err)
	fmt.Println("String: ", keySet.Keys.String())


	//any ideas?
}
