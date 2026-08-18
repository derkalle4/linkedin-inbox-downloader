package main

import (
	"fmt"
	"os"

	"github.com/derkalle4/linkedin-inbox-downloader/internal/app"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/termwin"
)

func main() {
	headless := false
	for _, a := range os.Args[1:] {
		if a == "--headless" {
			headless = true
			break
		}
	}

	if headless {
		if err := app.RunHeadless(); err != nil {
			os.Exit(1)
		}
		return
	}

	termwin.Setup()
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
