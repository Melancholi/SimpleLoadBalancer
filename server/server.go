package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	server_name := os.Getenv("SERVER_NAME")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Connected to: %s\n", server_name)
	})
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("Backend failure: ", err)
	}
}
