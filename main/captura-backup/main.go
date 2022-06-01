package main

import (
	"flag"
	"log"

	"captura-backup/internal/service"
)

func main() {
	info := flag.Bool("v", false, "will display the version of the program")
	flag.Parse()
	if *info {
		service.Version()
		return
	}
	srv, err := service.New()
	if err != nil {
		log.Fatalln("new service:", err)
	}

	srv.Start()
}
