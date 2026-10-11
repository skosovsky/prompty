package main

import (
	"context"
	"fmt"
	"log"

	hostdispatch "github.com/skosovsky/prompty/examples/host_dispatch"
)

func main() {
	text, err := hostdispatch.Demo(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(text)
}
