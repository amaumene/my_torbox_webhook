package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	"path/filepath"
)

// Define struct to hold the received JSON data

type Data struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

type Notification struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Data      Data      `json:"data"`
}

// Define structs to hold the API response data

type APIResponse struct {
	Success bool        `json:"success"`
	Error   interface{} `json:"error"`
	Detail  string      `json:"detail"`
	Data    []APIData   `json:"data"`
}

type APIData struct {
	ID                int       `json:"id"`
	Name              string    `json:"name"`
	CreatedAt         string    `json:"created_at"`
	UpdatedAt         string    `json:"updated_at"`
	AuthID            string    `json:"auth_id"`
	Hash              string    `json:"hash"`
	DownloadState     string    `json:"download_state"`
	DownloadSpeed     int       `json:"download_speed"`
	OriginalURL       interface{} `json:"original_url"`
	Eta               int       `json:"eta"`
	Progress          float64   `json:"progress"`
	Size              int64     `json:"size"`
	DownloadID        string    `json:"download_id"`
	Files             []APIFile `json:"files"`
	Active            bool      `json:"active"`
	Cached            bool      `json:"cached"`
	DownloadPresent   bool      `json:"download_present"`
	DownloadFinished  bool      `json:"download_finished"`
	ExpiresAt         string    `json:"expires_at"`
}

type APIFile struct {
	ID            int    `json:"id"`
	Md5           string `json:"md5"`
	Hash          string `json:"hash"`
	Name          string `json:"name"`
	Size          int64  `json:"size"`
	S3Path        string `json:"s3_path"`
	MimeType      string `json:"mimetype"`
	ShortName     string `json:"short_name"`
	AbsolutePath  string `json:"absolute_path"`
}

type DownloadResponse struct {
	Success bool        `json:"success"`
	Error   interface{} `json:"error"`
	Detail  string      `json:"detail"`
	Data    string      `json:"data"`
}

// Process the API calls and download in a separate goroutine
func processNotification(notification Notification) {
	// Define a regular expression to extract the desired string
	regexPattern := `download (.+?) has`
	re := regexp.MustCompile(regexPattern)

	// Find the desired string in the message
	match := re.FindStringSubmatch(notification.Data.Message)
	if len(match) < 2 {
		fmt.Println("Failed to extract the desired string")
		return
	}
	extractedString := match[1]

	// Perform the HTTP GET request to the API
	apiURL := "https://api.torbox.app/v1/api/usenet/mylist"
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		fmt.Println("Failed to create API request")
		return
	}

	// Set the Authorization header
	token := os.Getenv("API_TOKEN")
	if token == "" {
		fmt.Println("Environment variable API_TOKEN is not set.")
		os.Exit(1)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Failed to perform API request")
		return
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Failed to read API response")
		return
	}

	// Unmarshal the API response into an APIResponse struct
	var apiResponse APIResponse
	err = json.Unmarshal(respBody, &apiResponse)
	if err != nil {
		fmt.Println("Failed to parse API response")
		return
	}

	// Find the matching item and file in the API response
	var itemID, fileID int
	var shortName string
	for _, item := range apiResponse.Data {
		if item.Name == extractedString {
			for _, file := range item.Files {
				if strings.HasPrefix(file.MimeType, "video/") && !strings.Contains(file.ShortName, "sample") {
					itemID = item.ID
					fileID = file.ID
					shortName = file.ShortName
					break
				}
			}
			if itemID != 0 && fileID != 0 {
				break
			}
		}
	}

	if itemID == 0 || fileID == 0 {
		fmt.Println("No matching item found")
		return
	}

	// Print the extracted itemID, fileID, and shortName
	fmt.Printf("Extracted item ID: %d, file ID: %d, file short name: %s\n", itemID, fileID, shortName)

	// Make another API call with the itemID and fileID
	requestDLURL := fmt.Sprintf("https://api.torbox.app/v1/api/usenet/requestdl?token=%s&usenet_id=%d&file_id=%d&zip=false", token, itemID, fileID)

	req, err = http.NewRequest("GET", requestDLURL, nil)
	if err != nil {
		fmt.Println("Failed to create API request")
		return
	}

	resp, err = client.Do(req)
	if err != nil {
		fmt.Println("Failed to perform API request")
		return
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Failed to read API response")
		return
	}

	// Unmarshal the second API response into a DownloadResponse struct
	var downloadResponse DownloadResponse
	err = json.Unmarshal(respBody, &downloadResponse)
	if err != nil {
		fmt.Println("Failed to parse download API response")
		return
	}

	// Check if the download request was successful
	if !downloadResponse.Success {
		fmt.Println("Failed to request download")
		return
	}

	// Make the third API call to download the file content
	downloadURL := downloadResponse.Data
	resp, err = client.Get(downloadURL)
	if err != nil {
		fmt.Println("Failed to download file content")
		return
	}
	defer resp.Body.Close()

	downloadDir := os.Getenv("DOWNLOAD_DIR")
	if downloadDir == "" {
		log.Fatal("Environment variable DOWNLOAD_DIR is not set.")
	}
	fullFilePath := filepath.Join(downloadDir, shortName)

	// Create the file where the content will be streamed
	outFile, err := os.Create(fullFilePath)
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}
	defer outFile.Close()

	// Get the total size of the file for progress reporting
	totalSize := resp.ContentLength
	var downloadedSize int64

	// Create a buffer to hold chunks of data
	buf := make([]byte, 32*1024)

	// Write content to the file and show progress
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			// Write to file
			outFile.Write(buf[:n])

			// Update progress
			downloadedSize += int64(n)
			fmt.Printf("\rDownloading %s... %.2f%% complete", shortName, float64(downloadedSize)/float64(totalSize)*100)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Println("\nError while reading response body:", err)
			return
		}
	}

	fmt.Printf("\nFile downloaded and saved as %s\n", shortName)
}

// Handler function for the POST request
func handlePostData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Read the request body
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	// Unmarshal the JSON data into a Notification struct
	var notification Notification
	err = json.Unmarshal(body, &notification)
	if err != nil {
		http.Error(w, "Failed to parse JSON", http.StatusBadRequest)
		return
	}

	// Start processing in a separate goroutine
	go processNotification(notification)

	// Respond to the client immediately
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message": "Data received and processing started"}`))
}

func main() {
	http.HandleFunc("/api/data", handlePostData)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// Start the server
	port := ":3000"
	fmt.Printf("Server is running on port %s\n", port)
	log.Fatal(http.ListenAndServe(port, nil))
}
