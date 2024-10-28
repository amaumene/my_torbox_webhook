package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
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
	ID                int         `json:"id"`
	Name              string      `json:"name"`
	CreatedAt         string      `json:"created_at"`
	UpdatedAt         string      `json:"updated_at"`
	AuthID            string      `json:"auth_id"`
	Hash              string      `json:"hash"`
	DownloadState     string      `json:"download_state"`
	DownloadSpeed     int         `json:"download_speed"`
	OriginalURL       interface{} `json:"original_url"`
	Eta               int         `json:"eta"`
	Progress          float64     `json:"progress"`
	Size              int64       `json:"size"`
	DownloadID        string      `json:"download_id"`
	Files             []APIFile   `json:"files"`
	Active            bool        `json:"active"`
	Cached            bool        `json:"cached"`
	DownloadPresent   bool        `json:"download_present"`
	DownloadFinished  bool        `json:"download_finished"`
	ExpiresAt         string      `json:"expires_at"`
}

type APIFile struct {
	ID           int    `json:"id"`
	Md5          string `json:"md5"`
	Hash         string `json:"hash"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	S3Path       string `json:"s3_path"`
	MimeType     string `json:"mimetype"`
	ShortName    string `json:"short_name"`
	AbsolutePath string `json:"absolute_path"`
}

type DownloadResponse struct {
	Success bool        `json:"success"`
	Error   interface{} `json:"error"`
	Detail  string      `json:"detail"`
	Data    string      `json:"data"`
}

// Perform HTTP GET request
func performGetRequest(url, token string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create API request: %v", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform API request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read API response: %v", err)
	}

	return respBody, nil
}

// Process and download the file based on the notification
func processNotification(notification Notification) {
	extractedString, err := extractString(notification.Data.Message)
	if err != nil {
		log.Println("Error extracting string:", err)
		return
	}

	token := os.Getenv("API_TOKEN")
	if token == "" {
		log.Println("Environment variable API_TOKEN is not set.")
		return
	}

	// Perform the API call and get the response
	apiURL := "https://api.torbox.app/v1/api/usenet/mylist"
	respBody, err := performGetRequest(apiURL, token)
	if err != nil {
		log.Println(err)
		return
	}

	// Unmarshal the API response
	var apiResponse APIResponse
	err = json.Unmarshal(respBody, &apiResponse)
	if err != nil {
		log.Println("Failed to parse API response:", err)
		return
	}

	itemID, fileID, shortName, err := findMatchingItem(apiResponse, extractedString)
	if err != nil {
		log.Println(err)
		return
	}

	err = requestDownload(itemID, fileID, shortName, token)
	if err != nil {
		log.Println(err)
		return
	}
}

// Extract desired string from message
func extractString(message string) (string, error) {
	regexPattern := `download (.+?) has`
	re := regexp.MustCompile(regexPattern)
	match := re.FindStringSubmatch(message)
	if len(match) < 2 {
		return "", fmt.Errorf("failed to extract the desired string")
	}
	return match[1], nil
}

// Find matching item in API response
func findMatchingItem(apiResponse APIResponse, extractedString string) (int, int, string, error) {
	for _, item := range apiResponse.Data {
		if item.Name == extractedString {
			for _, file := range item.Files {
				if strings.HasPrefix(file.MimeType, "video/") && !strings.Contains(file.ShortName, "sample") {
					return item.ID, file.ID, file.ShortName, nil
				}
			}
		}
	}
	return 0, 0, "", fmt.Errorf("no matching item found")
}

// Request download using itemID, fileID and token
func requestDownload(itemID, fileID int, shortName, token string) error {
	requestDLURL := fmt.Sprintf("https://api.torbox.app/v1/api/usenet/requestdl?token=%s&usenet_id=%d&file_id=%d&zip=false", token, itemID, fileID)

	respBody, err := performGetRequest(requestDLURL, token)
	if err != nil {
		return err
	}

	var downloadResponse DownloadResponse
	err = json.Unmarshal(respBody, &downloadResponse)
	if err != nil {
		return fmt.Errorf("failed to parse download API response: %v", err)
	}

	if !downloadResponse.Success {
		return fmt.Errorf("failed to request download")
	}

	err = downloadFile(downloadResponse.Data, shortName)
	if err != nil {
		return err
	}

	return nil
}

// Download file from URL and save to specified path
func downloadFile(downloadURL, shortName string) error {
	client := &http.Client{}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download file content: %v", err)
	}
	defer resp.Body.Close()

	downloadDir := os.Getenv("DOWNLOAD_DIR")
	if downloadDir == "" {
		log.Fatal("Environment variable DOWNLOAD_DIR is not set.")
	}
	fullFilePath := filepath.Join(downloadDir, shortName)

	outFile, err := os.Create(fullFilePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer outFile.Close()

	writeContentToFile(resp, outFile, shortName, resp.ContentLength)

	fmt.Printf("\nFile downloaded and saved as %s\n", shortName)
	return nil
}

// Write content to file and show progress
func writeContentToFile(resp *http.Response, outFile *os.File, shortName string, totalSize int64) {
	var downloadedSize int64
	buf := make([]byte, 32*1024)

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			outFile.Write(buf[:n])
			downloadedSize += int64(n)
			fmt.Printf("\rDownloading %s... %.2f%% complete", shortName, float64(downloadedSize)/float64(totalSize)*100)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Println("\nError while reading response body:", err)
			return
		}
	}
}

// Handler function for the POST request
func handlePostData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	var notification Notification
	err = json.Unmarshal(body, &notification)
	if err != nil {
		http.Error(w, "Failed to parse JSON", http.StatusBadRequest)
		return
	}

	go processNotification(notification)

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message": "Data received and processing started"}`))
}

// Monitor new files in specified directory
func monitorNewFiles(watchDirectory string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Create == fsnotify.Create {
					fmt.Println("Detected new file:", event.Name)
					if err := processFile(event.Name); err != nil {
						log.Println("Error processing file:", err)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("Error:", err)
			}
		}
	}()

	if err := watcher.Add(watchDirectory); err != nil {
		log.Fatal(err)
	}

	<-done
}

// Process a newly created file
func processFile(fileName string) error {
	token := os.Getenv("API_TOKEN")
	if token == "" {
		return fmt.Errorf("environment variable API_TOKEN is not set")
	}

	fullFilePath, err := filepath.Abs(fileName)
	if err != nil {
		return fmt.Errorf("could not get absolute path of file: %w", err)
	}

	file, err := os.Open(fullFilePath)
	if err != nil {
		return fmt.Errorf("could not open file: %w", err)
	}
	defer file.Close()

	return uploadFileWithRetries(fullFilePath, file, filepath.Base(fileName), token)
}

// Upload a file to the server
func uploadFile(file *os.File, fileName, token string) ([]byte, int, error) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	fw, err := w.CreateFormFile("file", fileName)
	if err != nil {
		return nil, 0, fmt.Errorf("could not create form file: %w", err)
	}
	if _, err = io.Copy(fw, file); err != nil {
		return nil, 0, fmt.Errorf("could not copy file contents: %w", err)
	}
	w.Close()

	req, err := http.NewRequest("POST", "https://api.torbox.app/v1/api/usenet/createusenetdownload", &b)
	if err != nil {
		return nil, 0, fmt.Errorf("could not create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("could not perform HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("could not read response body: %w", err)
	}

	return body, resp.StatusCode, nil
}

// Handle the response of the file upload with multiple attempts
func uploadFileWithRetries(fullFilePath string, file *os.File, fileName, token string) error {
	const maxAttempts = 5
	delay := 2 * time.Second

	for attempts := 0; attempts < maxAttempts; attempts++ {
		response, statusCode, err := uploadFile(file, fileName, token)
		if err != nil {
			fmt.Printf("Attempt %d: error uploading file: %v\n", attempts+1, err)
			time.Sleep(delay)
			continue
		}

		respString := string(response)
		if statusCode == http.StatusOK && strings.Contains(respString, "success") {
			fmt.Println("File uploaded successfully:", fileName)
			if err := os.Remove(fullFilePath); err != nil {
				fmt.Printf("Attempt %d: could not delete file: %v\n", attempts+1, err)
			} else {
				fmt.Println("File deleted:", fileName)
			}
			return nil
		} else {
			fmt.Printf("Attempt %d: failed to upload file. Status code: %d. Response: %s\n", attempts+1, statusCode, respString)
			time.Sleep(5 * time.Second)
		}
	}

	return fmt.Errorf("retry limit exceeded. Failed to upload file: %s", fullFilePath)
}

func main() {
	go func() {
		incomingDir := os.Getenv("INCOMING_NZB")
		if incomingDir == "" {
			log.Fatal("INCOMING_NZB environment variable is not set")
		}
		monitorNewFiles(incomingDir)
	}()

	http.HandleFunc("/api/data", handlePostData)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	port := ":3000"
	fmt.Printf("Server is running on port %s\n", port)
	log.Fatal(http.ListenAndServe(port, nil))
}