package main

import (
	"log"
	"net/http"
	"smcp/database"
	"smcp/server"
)

func main() {
	mux := http.NewServeMux()

	server.InitServer(mux)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	dbService, err := database.NewDatabaseService("../data/app.db")
	if err != nil {
		panic("Failed to connect to the database")
	}
	defer dbService.Close()

	err = dbService.Init()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("listening on :8080")
	log.Fatal(server.ListenAndServe())
}
