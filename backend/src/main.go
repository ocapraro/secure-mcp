package main

import (
	"log"
	"net/http"
	"smcp/database"
	"smcp/server"
)

func main() {
	dbService, err := database.NewDatabaseService("../data/app.db")
	if err != nil {
		panic("Failed to connect to the database")
	}
	defer dbService.Close()

	err = dbService.Init()
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()

	server.InitServer(mux, dbService)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	log.Println("listening on :8080")
	log.Fatal(server.ListenAndServe())
}
