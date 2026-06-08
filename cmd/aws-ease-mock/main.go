package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
)

type response struct {
	Route   string              `json:"route"`
	Path    string              `json:"path"`
	Query   map[string][]string `json:"query"`
	Method  string              `json:"method"`
	Payload string              `json:"payload"`
}

func main() {
	addr := os.Getenv("AWS_EASE_MOCK_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/lambda/", echo("lambda"))
	mux.HandleFunc("/sqs/", echo("sqs"))

	log.Printf("aws-ease mock listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func echo(route string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		payload, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resp := response{
			Route:   route,
			Path:    r.URL.Path,
			Query:   r.URL.Query(),
			Method:  r.Method,
			Payload: string(payload),
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
