/*
Purpose: Start the KubeSift desktop executable.
Used by: The desktop build and macOS app bundle.
Responsibilities: Load the bundled frontend and start the thin desktop adapter.
Important: Startup must stay local; opening the app must not contact Kubernetes or run a credential helper.
*/
// KubeSift Phase 1A desktop entry point.
package main

import (
	"github.com/sandjaie/KubeSift/internal/desktop"
	"github.com/sandjaie/KubeSift/internal/desktop/assets"
	"log"
)

func main() {
	if err := desktop.Run(assets.Files()); err != nil {
		log.Fatal(err)
	}
}
