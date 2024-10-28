package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

const (
	apiURL       = "https://api.torbox.app/v1/api/usenet/mylist"
	requestDLURL = "https://api.torbox.app/v1/api/usenet/requestdl"
	uploadURL    = "https://api.torbox.app/v1/api/usenet/createusenetdownload"
)

var (
	apiToken    string
	downloadDir string
	incomingDir string
	httpClient  = &http.Client{}
)

func init() {
	apiToken = os.Getenv("API_TOKEN")
	if apiToken == "" {
		log.Fatal("Environment variable API_TOKEN is not set.")
	}

	downloadDir = os.Getenv("DOWNLOAD_DIR")
	if downloadDir == "" {
		log.Fatal("Environment variable DOWNLOAD_DIR is not set.")
	}

	incomingDir = os.Getenv("INCOMING_NZB")
	if incomingDir == "" {
		log.Fatal("INCOMING_NZB environment variable is not set.")
	}
}

func main() {
	go monitorNewFiles(incomingDir)

	http.HandleFunc("/api/data", handlePostData)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	port := ":3000"
	fmt.Printf("Server is running on port %s\n", port)
	log.Fatal(http.ListenAndServe(port, nil))
}