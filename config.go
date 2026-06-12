package main

import (
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	defaultModel   = "deepseek-v4-flash" // আপনার পছন্দের নতুন মডেল এখানে সেট করা হলো
	apiBase        = "https://api.tokenlab.sh/v1"
	maxHistory     = 20
	requestTimeout = 90
)

var (
	sessions   = make(map[string]*UserSession)
	sessionsMu sync.RWMutex

	apiKey       string
	tavilyKey    string
	systemPrompt string
	httpClient   = &http.Client{Timeout: time.Duration(requestTimeout) * time.Second}
)

func initConfig() {
	apiKey = os.Getenv("TOKENLAB_API_KEY")
	if apiKey == "" {
		log.Fatal("❌ TOKENLAB_API_KEY environment variable is required")
	}

	tavilyKey = os.Getenv("TAVILY_API_KEY")
	if tavilyKey == "" {
		log.Println("⚠️ TAVILY_API_KEY missing! Live web search will be disabled.")
	}

	systemPrompt = os.Getenv("SYSTEM_PROMPT")
}
