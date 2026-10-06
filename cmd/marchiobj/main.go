package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/marchi/marchiobj/httpapi"
	"github.com/marchi/marchiobj/store"
)

func main() {
	addr := flag.String("addr", ":7300", "listen address")
	dataDir := flag.String("dataDir", "./data", "data directory")
	flag.Parse()

	st, err := store.Open(*dataDir)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	srv := httpapi.New(st)
	log.Printf("marchiobj listening on %s dataDir=%s", *addr, *dataDir)
	if err := http.ListenAndServe(*addr, srv); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
