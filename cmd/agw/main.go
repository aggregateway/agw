package main

import (
	"log"
	"os"

	"github.com/aggregateway/agw"
)

func main() {
	if err := agw.Run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
