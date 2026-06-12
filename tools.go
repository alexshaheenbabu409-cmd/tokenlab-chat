package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func SearchInternet(query string) string {
	if tavilyKey == "" {
		return "Search API key is not configured."
	}

	payload := TavilyRequest{
		APIKey:      tavilyKey,
		Query:       query,
		SearchDepth: "basic",
		MaxResults:  3,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ""
	}

	resp, err := http.Post("https://api.tavily.com/search", "application/json", bytes.NewReader(body))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var result TavilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}

	if len(result.Results) == 0 {
		return "No relevant recent information found on the internet."
	}

	var sb strings.Builder
	sb.WriteString("Here is the latest live internet data for context:\n")
	for i, res := range result.Results {
		sb.WriteString(fmt.Sprintf("[%d] Title: %s\nContent: %s\nURL: %s\n\n", i+1, res.Title, res.Content, res.URL))
	}
	return sb.String()
}

func ShouldSearchQuery(msg string) bool {
	msg = strings.ToLower(msg)
	keywords := []string{
		"বর্তমান", "এখন", "আজকের", "প্রধানমন্ত্রী", "প্রেসিডেন্ট", "নির্বাচন", 
		"আবহাওয়া", "খেলা", "খবর", "দাম", "সোনার দাম", "ক্রিপ্টো", "বিটকয়েন", 
		"current", "today", "now", "latest", "price", "gold price", "crypto", "bitcoin", "news",
	}

	for _, kw := range keywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}
