package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

const (
	apiURL       = "https://api.torbox.app/v1/api/usenet/mylist"
	requestDLURL = "https://api.torbox.app/v1/api/usenet/requestdl"
	uploadURL    = "https://api.torbox.app/v1/api/usenet/createusenetdownload"
	controlURL   = "https://api.torbox.app/v1/api/usenet/controlusenetdownload"
)

var (
	apiToken    string
	downloadDir string
	nzbDir      string
	tempDir     string
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

	nzbDir = os.Getenv("NZB_DIR")
	if nzbDir == "" {
		log.Fatal("NZB_DIR environment variable is not set.")
	}

	tempDir = os.Getenv("TEMP_DIR")
	if tempDir == "" {
		log.Fatal("TEMP_DIR environment variable is not set")
	}
	cleanDir(tempDir)
}

func cleanDir(tempDir string) {
	files, err := os.ReadDir(tempDir)
	if err != nil {
		log.Fatalf("Failed to read temp directory: %v", err)
	}

	for _, file := range files {
		err := os.RemoveAll(filepath.Join(tempDir, file.Name()))
		if err != nil {
			log.Printf("Failed to remove file %s: %v", file.Name(), err)
		}
	}
}

func main() {
	go monitorNewFiles(nzbDir)

	http.HandleFunc("/api/data", handlePostData)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	port := ":3000"
	fmt.Printf("Server is running on port %s\n", port)
	log.Fatal(http.ListenAndServe(port, nil))
}
