package main

import (
	"log"

	"github.com/PetoAdam/homenavi/matter-commissioner/internal/app"
)

func main() {
	if err := app.Run(app.LoadConfig()); err != nil {
		log.Fatal(err)
	}
}