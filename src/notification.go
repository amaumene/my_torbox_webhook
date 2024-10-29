package main

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"time"
)

type Notification struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Data      Data      `json:"data"`
}

type Data struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

func processNotification(notification Notification) {
	extractedString, err := extractString(notification.Data.Message)
	if err != nil {
		log.Println("Error extracting string:", err)
		return
	}

	respBody, err := performGetRequest(apiURL, apiToken)
	if err != nil {
		log.Println(err)
		return
	}

	var apiResponse APIResponse
	err = json.Unmarshal(respBody, &apiResponse)
	if err != nil {
		log.Println("Failed to parse API response:", err)
		return
	}

	itemID, fileID, fileSize, shortName, err := findMatchingItem(apiResponse, extractedString)
	if err != nil {
		log.Println(err)
		return
	}

	err = requestDownload(itemID, fileID, fileSize, shortName, apiToken)
	if err != nil {
		log.Println(err)
		return
	}
}

func extractString(message string) (string, error) {
	regexPattern := `download (.+?) has`
	re := regexp.MustCompile(regexPattern)
	match := re.FindStringSubmatch(message)
	if len(match) < 2 {
		return "", fmt.Errorf("failed to extract the desired string")
	}
	return match[1], nil
}