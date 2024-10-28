package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

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

func processFile(fileName string) error {
	fullFilePath, err := filepath.Abs(fileName)
	if err != nil {
		return fmt.Errorf("could not get absolute path of file: %w", err)
	}

	file, err := os.Open(fullFilePath)
	if err != nil {
		return fmt.Errorf("could not open file: %w", err)
	}
	defer file.Close()

	return uploadFileWithRetries(fullFilePath, file, filepath.Base(fileName), apiToken)
}

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

	req, err := http.NewRequest("POST", uploadURL, &b)
	if err != nil {
		return nil, 0, fmt.Errorf("could not create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
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