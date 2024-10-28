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

	"github.com/fsnotify/fsnotify"
)

const maxRetries = 3
const retryDelay = 2 * time.Second

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

    fmt.Println("File uploaded successfully:", filePath)

    // Delete the file after successful upload
    err = os.Remove(filePath)
    if err != nil {
        return fmt.Errorf("failed to delete file: %v", err)
    }
    return nil
}