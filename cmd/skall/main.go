package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/Harish-vinayagam/Skall/internal/network"
)

func main() {
	mode := flag.String("mode", "server", "server or client")
	address := flag.String("address", "localhost:4000", "TCP address")

	flag.Parse()

	switch *mode {
	case "server":
		if err := network.StartServer(*address); err != nil {
			log.Fatal(err)
		}
	case "client":
		if err := network.StartClient(*address); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Println("Usage:")
		fmt.Println("  skall -mode=server -address=localhost:4000")
		fmt.Println("  skall -mode=client -address=localhost:4000")
	}
}
