package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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

func requestDownload(itemID, fileID int, shortName, token string) error {
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

	return downloadFile(downloadResponse.Data, shortName)
}

func downloadFile(downloadURL, shortName string) error {
	resp, err := httpClient.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download file content: %v", err)
	}
	defer resp.Body.Close()

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