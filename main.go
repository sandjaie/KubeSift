// KubeSift Phase 1A desktop entry point.
package main

import (
	"embed"
	"github.com/sandjaie/KubeSift/internal/desktop"
	"log"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := desktop.Run(assets); err != nil {
		log.Fatal(err)
	}
}
