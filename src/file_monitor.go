package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"strings"
	"encoding/json"

	"github.com/fsnotify/fsnotify"
)

const maxRetries = 3
const retryDelay = 2 * time.Second

var serverResponse struct {
    Success bool `json:"success"`
    Detail  string `json:"detail"`
    Data struct {
        Hash             string `json:"hash"`
        UsenetDownloadID int    `json:"usenetdownload_id"`
        AuthID           string `json:"auth_id"`
    } `json:"data"`
}

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
					if err := uploadFileWithRetries(event.Name); err != nil {
						log.Println("Error uploading file:", err)
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

func uploadFileWithRetries(filePath string) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = uploadFile(filePath)
		if err == nil {
			return nil
		}
		log.Printf("Upload failed: %v. Retrying in %v...\n", err, retryDelay)
		time.Sleep(retryDelay)
	}
	return fmt.Errorf("failed to upload file after %d attempts: %v", maxRetries, err)
}

func uploadFile(filePath string) error {
    file, err := os.Open(filePath)
    if err != nil {
        return fmt.Errorf("failed to open file: %v", err)
    }
    defer file.Close()

    body := &bytes.Buffer{}
    writer := multipart.NewWriter(body)
    part, err := writer.CreateFormFile("file", filepath.Base(file.Name()))
    if err != nil {
        return fmt.Errorf("failed to create form file: %v", err)
    }

    _, err = io.Copy(part, file)
    if err != nil {
        return fmt.Errorf("failed to copy file content: %v", err)
    }

    // Add the name field without the extension
    fileNameWithoutExt := strings.TrimSuffix(filepath.Base(file.Name()), filepath.Ext(file.Name()))
    err = writer.WriteField("name", fileNameWithoutExt)
    if err != nil {
        return fmt.Errorf("failed to write name field: %v", err)
    }

    err = writer.Close()
    if err != nil {
        return fmt.Errorf("failed to close writer: %v", err)
    }

    req, err := http.NewRequest("POST", uploadURL, body)
    if err != nil {
        return fmt.Errorf("failed to create request: %v", err)
    }
    req.Header.Set("Content-Type", writer.FormDataContentType())
    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))

    resp, err := httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("failed to upload file: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("failed to upload file, status: %s", resp.Status)
    }

    // Read and print the response body
    respBody, err := io.ReadAll(resp.Body)
    if err != nil {
        return fmt.Errorf("failed to read response body: %v", err)
    }

    err = json.Unmarshal(respBody, &serverResponse)
    if err != nil {
        return fmt.Errorf("failed to parse response body: %v", err)
    }

    if serverResponse.Success != true {
        return fmt.Errorf("failed to upload file: %s", serverResponse.Detail)
    }
    fmt.Println("File uploaded successfully:", filePath)

    fmt.Println("Response from server:", string(respBody))
    // Delete the file after successful upload
    err = os.Remove(filePath)
    if err != nil {
        return fmt.Errorf("failed to delete file: %v", err)
    }

    if serverResponse.Detail == "Found cached usenet download. Using cached download." {
    	respBody, err := performGetRequest(apiURL, apiToken)
	    if err != nil {
		    return fmt.Errorf("failed to perform API request: %v", err)
	    }

	    var apiResponse APIResponse
	    err = json.Unmarshal(respBody, &apiResponse)
	    if err != nil {
		    return fmt.Errorf("failed to parse API response: %v", err)
	    }
        itemID, fileID, fileSize, shortName, err := findMatchingItemID(apiResponse, serverResponse.Data.UsenetDownloadID)
	    if err != nil {
		    return fmt.Errorf("failed to find matching item: %v", err)
	    }

	    err = requestDownload(itemID, fileID, fileSize, shortName, apiToken)
	    if err != nil {
		    return fmt.Errorf("failed to request download: %v", err)
	    }
    }

    return nil
}