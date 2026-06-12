package main

import (
	"sync"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	Temperature float64   `json:"temperature,omitempty"`
}

type ChatResponse struct {
	Choices []struct {
		Delta   *Message `json:"delta"`
		Message *Message `json:"message"`
	} `json:"choices"`
	Error *APIError `json:"error"`
}

type APIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code"`
}

type UserSession struct {
	History []Message
	mu      sync.Mutex
}

type TavilyRequest struct {
	APIKey      string `json:"api_key"`
	Query       string `json:"query"`
	SearchDepth string `json:"search_depth,omitempty"`
	MaxResults  int    `json:"max_results,omitempty"`
}

type TavilyResponse struct {
	Results []struct {
		Title   string  `json:"title"`
		URL     string  `json:"url"`
		Content string  `json:"content"`
		Score   float64 `json:"score"`
	} `json:"results"`
}
