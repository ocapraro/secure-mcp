package main

import (
	"log"
	"net/http"
	"smcp/server"
)

func main() {
	mux := http.NewServeMux()

	server.InitServer(mux)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	log.Println("listening on :8080")
	log.Fatal(server.ListenAndServe())
}
