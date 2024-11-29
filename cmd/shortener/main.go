package main

import (
	"fmt"
	"net/http"

	"github.com/BrownBear56/contractor/cmd/shortener/handlers"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handlers.PostHandler(w, r)
		} else if r.Method == http.MethodGet {
			handlers.GetHandler(w, r)
		} else {
			http.Error(w, "Unsupported method", http.StatusBadRequest)
		}
	})

	fmt.Println("Server is running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}
