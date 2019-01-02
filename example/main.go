package main

import (
	"fmt"
	"github.com/jugelizidemo/minicache"
	"time"
)

func main() {
	defaultExpiration, _ := time.ParseDuration("0.5h")
	gcInterval, _ := time.ParseDuration("3s")
	minic := minicache.NewMiniCache(defaultExpiration, gcInterval)

	expiration, _ := time.ParseDuration("2s")
	k1 := "golang"
	minic.Set("golang", k1, expiration)

	if v, found := minic.Get("golang"); found {
		fmt.Println("found golang:", v)
	} else {
		fmt.Println("not found golang")
	}

	err := minic.SaveToFile("./golang.txt")
	if err != nil {
		fmt.Println(err)
	}
	err = minic.LoadFromFile("./golang.txt")
	if err != nil {
		fmt.Println(err)
	}

	s, _ := time.ParseDuration("1.999s")
	time.Sleep(s)
	if v, found := minic.Get("golang"); found {
		fmt.Println("found golang:", v)
	} else {
		fmt.Println("not found golang")
	}

}
