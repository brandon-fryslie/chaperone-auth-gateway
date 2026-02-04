package main

import (
	"log"
	"os"

	"github.com/bmf/chaperone/cmd/chaperone/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
