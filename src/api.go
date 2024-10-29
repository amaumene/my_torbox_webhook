package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"sync"
	"bytes"
)

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

func performGetRequest(url, token string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create API request: %v", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform API request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read API response: %v", err)
	}

	return respBody, nil
}

func findMatchingItem(apiResponse APIResponse, extractedString string) (int, int, int64, string, error) {
	for _, item := range apiResponse.Data {
		if item.Name == extractedString {
			for _, file := range item.Files {
				if strings.HasPrefix(file.MimeType, "video/") && !strings.Contains(file.ShortName, "sample") {
					return item.ID, file.ID, file.Size, file.ShortName, nil
				}
			}
		}
	}
	return 0, 0, 0, "", fmt.Errorf("no matching item found")
}

func findMatchingItemID(apiResponse APIResponse, itemID int) (int, int, int64, string, error) {
    for _, item := range apiResponse.Data {
        if item.ID == itemID {
            for _, file := range item.Files {
                if strings.HasPrefix(file.MimeType, "video/") && !strings.Contains(file.ShortName, "sample") {
                    return item.ID, file.ID, file.Size, file.ShortName, nil
                }
            }
        }
    }
    return 0, 0, 0, "", fmt.Errorf("no matching item found for ID %d", itemID)
}

func requestDownload(itemID, fileID int, fileSize int64, shortName, token string) error {
	url := fmt.Sprintf("%s?token=%s&usenet_id=%d&file_id=%d&zip=false", requestDLURL, token, itemID, fileID)

	respBody, err := performGetRequest(url, token)
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

	err = downloadFile(downloadResponse.Data, shortName, fileSize)
	if err != nil {
	    return fmt.Errorf("failed to download file: %v", err)
    }
    return deleteFile(itemID, token)
}

func deleteFile(itemID int, token string) error {
	data := map[string]interface{}{
		"usenet_id": itemID,
		"operation": "delete",
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %v", err)
	}

	req, err := http.NewRequest("POST", controlURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to perform request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete file, status: %s", resp.Status)
	}
    fmt.Printf("File deleted successfully\n")
	return nil
}

func downloadFile(downloadURL, shortName string, fileSize int64) error {
	resp, err := httpClient.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download file content: %v", err)
	}
	defer resp.Body.Close()

	writeContentToFile(resp, shortName, fileSize)

	fmt.Printf("\nFile downloaded and saved as %s\n", shortName)
	return nil
}

func writeContentToFile(resp *http.Response, shortName string, totalSize int64) error {
	tempFile, err := os.CreateTemp(tempDir, "tempfile-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer tempFile.Close()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var totalDownloaded int64
	chunkSize := totalSize / 4 // Number of chunks to split the file into
	startTime := time.Now()

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			start := int64(i) * chunkSize
			end := start + chunkSize
			if i == 3 {
				end = totalSize
			}

			if err := downloadChunk(resp.Request.URL.String(), start, end, tempFile, &mu, &totalDownloaded, startTime, shortName, totalSize); err != nil {
				log.Println("Error downloading chunk:", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify if the downloaded file size matches the expected total size
	fileInfo, err := tempFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get temporary file info: %v", err)
	}
	if fileInfo.Size() != totalSize {
		log.Println("Downloaded file size does not match the expected size, restarting download...")
        return downloadFile(resp.Request.URL.String(), shortName, totalSize)
	}

	finalFilePath := filepath.Join(downloadDir, shortName)
	if err := os.Rename(tempFile.Name(), finalFilePath); err != nil {
		return fmt.Errorf("failed to rename temporary file: %v", err)
	}

	return nil
}

func downloadChunk(url string, start, end int64, tempFile *os.File, mu *sync.Mutex, totalDownloaded *int64, startTime time.Time, shortName string, totalSize int64) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end-1))
	req.Proto = "HTTP/2.0"

	partResp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error performing request: %v", err)
	}
	defer partResp.Body.Close()

	partBuf := make([]byte, 32*1024)
	for {
		n, err := partResp.Body.Read(partBuf)
		if n > 0 {
			mu.Lock()
			if _, writeErr := tempFile.WriteAt(partBuf[:n], start); writeErr != nil {
				mu.Unlock()
				return fmt.Errorf("error writing to temporary file: %v", writeErr)
			}
			start += int64(n)
			*totalDownloaded += int64(n)
			elapsedTime := time.Since(startTime).Seconds()
			speed := float64(*totalDownloaded) / elapsedTime / 1024 // speed in KB/s
			fmt.Printf("\rDownloading %s... %.2f%% complete, Speed: %.2f KB/s", shortName, float64(*totalDownloaded)/float64(totalSize)*100, speed)
			mu.Unlock()
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading response body: %v", err)
		}
	}
	return nil
}