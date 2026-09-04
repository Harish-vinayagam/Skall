package main

import (
	"fmt"
	"log"
	"os"

	"github.com/Harish-vinayagam/Skall/internal/identity"

	"github.com/Harish-vinayagam/Skall/internal/network"
)

func main() {
	localIdentity, err := loadLocalIdentity()
	if err != nil {
		log.Fatal(err)
	}

	args := os.Args[1:]
	if len(args) == 0 {
		if err := network.StartServer("localhost:4000", localIdentity); err != nil {
			log.Fatal(err)
		}
		return
	}

	switch args[0] {
	case "identity":
		fmt.Println(localIdentity.Summary())
	case "server":
		address := "localhost:4000"
		if len(args) > 1 {
			address = args[1]
		}
		if err := network.StartServer(address, localIdentity); err != nil {
			log.Fatal(err)
		}
	case "client":
		address := "localhost:4000"
		if len(args) > 1 {
			address = args[1]
		}
		if err := network.StartClient(address, localIdentity); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Println("Usage:")
		fmt.Println("  skall identity")
		fmt.Println("  skall server [address]")
		fmt.Println("  skall client [address]")
	}
}

func loadLocalIdentity() (identity.Identity, error) {
	store, err := identity.DefaultStore()
	if err != nil {
		return identity.Identity{}, err
	}

	return store.LoadOrCreate()
}


