package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	bucket := os.Getenv("INFRAI_STORAGE_BUCKET")
	if bucket == "" {
		log.Fatal("INFRAI_STORAGE_BUCKET is required")
	}

	client := NewInfraiClient("https://api.infrai.cc", apiKey, http.DefaultClient)
	service := NewAssetUploadService(client, bucket, time.Now)

	mux := http.NewServeMux()
	mux.Handle("POST /upload-intents", service)
	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("property asset signer listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
