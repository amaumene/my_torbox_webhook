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
		return fmt.Errorf("error extracting string: %v", err)
	}

	respBody, err := performGetRequest(apiURL, apiToken)
	if err != nil {
	    return fmt.Errorf("failed to read API response: %v", err)
	}

	var apiResponse APIResponse
	err = json.Unmarshal(respBody, &apiResponse)
	if err != nil {
	    return fmt.Errorf("failed to parse API response: %v", err)
	}

	itemID, fileID, fileSize, shortName, err := findMatchingItem(apiResponse, extractedString)
	if err != nil {
	    return fmt.Errorf("failed to find matching item: %v", err)
	}

	err = requestDownload(itemID, fileID, fileSize, shortName, apiToken)
	if err != nil {
	    return fmt.Errorf("failed to request download: %v", err)
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