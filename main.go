package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ─── Config ───────────────────────────────────────────────────────────────────

const (
	defaultModel   = "gpt-4.1"
	apiBase        = "https://api.tokenlab.sh/v1"
	maxHistory     = 20   // per user (pairs)
	requestTimeout = 90   // seconds
)

// ─── Types ────────────────────────────────────────────────────────────────────

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	Temperature float64   `json:"temperature,omitempty"`
	Tools       []Tool    `json:"tools,omitempty"`
}

type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

type Function struct {
	Name        string `json:"name"`
	Description string `json:"description"`
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

// UserSession holds per-user conversation history
type UserSession struct {
	History []Message
	mu      sync.Mutex
}

// ─── Global State ─────────────────────────────────────────────────────────────

var (
	sessions   = make(map[string]*UserSession)
	sessionsMu sync.RWMutex

	apiKey     string
	systemPrompt string
	httpClient = &http.Client{Timeout: time.Duration(requestTimeout) * time.Second}
)

// ─── Session Management ───────────────────────────────────────────────────────

func getSession(userID string) *UserSession {
	sessionsMu.RLock()
	s, ok := sessions[userID]
	sessionsMu.RUnlock()
	if ok {
		return s
	}
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	s = &UserSession{}
	sessions[userID] = s
	return s
}

func (s *UserSession) addMessage(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.History = append(s.History, Message{Role: role, Content: content})
	// sliding window: keep last maxHistory pairs
	if len(s.History) > maxHistory*2 {
		s.History = s.History[len(s.History)-maxHistory*2:]
	}
}

func (s *UserSession) getMessages(userMsg string) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := make([]Message, len(s.History)+1)
	copy(msgs, s.History)
	msgs[len(s.History)] = Message{Role: "user", Content: userMsg}
	return msgs
}

func (s *UserSession) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.History = nil
}

// ─── API Call ─────────────────────────────────────────────────────────────────

// webSearchTool returns the web_search tool definition for gpt-4.1
func webSearchTool() Tool {
	return Tool{
		Type: "function",
		Function: Function{
			Name:        "web_search",
			Description: "Search the web for real-time information",
		},
	}
}

func callAPI(messages []Message, model string, stream bool) (*http.Response, error) {
	payload := ChatRequest{
		Model:       model,
		Messages:    messages,
		Stream:      stream,
		Temperature: 0.7,
	}

	// enable web search for gpt-4.1
	if strings.HasPrefix(model, "gpt-4.1") {
		payload.Tools = []Tool{webSearchTool()}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal error: %w", err)
	}

	req, err := http.NewRequest("POST", apiBase+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("request build error: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}
	return resp, nil
}

// ─── HTTP Handlers ────────────────────────────────────────────────────────────

// POST /chat  — streaming SSE response
// Body: {"user_id":"...", "message":"...", "model":"..."}
func handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID  string `json:"user_id"`
		Message string `json:"message"`
		Model   string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	req.UserID = strings.TrimSpace(req.UserID)
	req.Message = strings.TrimSpace(req.Message)
	if req.UserID == "" {
		req.UserID = "default"
	}
	if req.Message == "" {
		http.Error(w, `{"error":"message is required"}`, http.StatusBadRequest)
		return
	}
	if req.Model == "" {
		req.Model = defaultModel
	}

	// special command
	if req.Message == "/clear" {
		getSession(req.UserID).clear()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"reply": "✅ History cleared!"})
		return
	}

	session := getSession(req.UserID)
	messages := session.getMessages(req.Message)

	// prepend system prompt
	if systemPrompt != "" {
		full := make([]Message, 0, len(messages)+1)
		full = append(full, Message{Role: "system", Content: systemPrompt})
		full = append(full, messages...)
		messages = full
	}

	resp, err := callAPI(messages, req.Model, true)
	if err != nil {
		log.Printf("API error for user %s: %v", req.UserID, err)
		http.Error(w, `{"error":"upstream API error"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("API non-200 (%d): %s", resp.StatusCode, body)
		http.Error(w, fmt.Sprintf(`{"error":"API returned %d"}`, resp.StatusCode), http.StatusBadGateway)
		return
	}

	// Stream SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, canFlush := w.(http.Flusher)

	var fullReply strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk ChatResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", chunk.Error.Message)
			if canFlush {
				flusher.Flush()
			}
			return
		}

		for _, choice := range chunk.Choices {
			if choice.Delta != nil && choice.Delta.Content != "" {
				token := choice.Delta.Content
				fullReply.WriteString(token)
				fmt.Fprintf(w, "data: %s\n\n", jsonString(token))
				if canFlush {
					flusher.Flush()
				}
			}
		}
	}

	// save to history
	if fullReply.Len() > 0 {
		session.addMessage("user", req.Message)
		session.addMessage("assistant", fullReply.String())
	}

	fmt.Fprint(w, "data: [DONE]\n\n")
	if canFlush {
		flusher.Flush()
	}
}

// POST /chat/simple — non-streaming JSON response
func handleSimpleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID  string `json:"user_id"`
		Message string `json:"message"`
		Model   string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	req.UserID = strings.TrimSpace(req.UserID)
	req.Message = strings.TrimSpace(req.Message)
	if req.UserID == "" {
		req.UserID = "default"
	}
	if req.Message == "" {
		http.Error(w, `{"error":"message is required"}`, http.StatusBadRequest)
		return
	}
	if req.Model == "" {
		req.Model = defaultModel
	}

	session := getSession(req.UserID)
	messages := session.getMessages(req.Message)

	if systemPrompt != "" {
		full := make([]Message, 0, len(messages)+1)
		full = append(full, Message{Role: "system", Content: systemPrompt})
		full = append(full, messages...)
		messages = full
	}

	resp, err := callAPI(messages, req.Model, false)
	if err != nil {
		jsonError(w, "upstream API error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	var result ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		jsonError(w, "failed to parse API response", http.StatusInternalServerError)
		return
	}
	if result.Error != nil {
		jsonError(w, result.Error.Message, http.StatusBadGateway)
		return
	}

	reply := ""
	if len(result.Choices) > 0 && result.Choices[0].Message != nil {
		reply = result.Choices[0].Message.Content
	}

	session.addMessage("user", req.Message)
	session.addMessage("assistant", reply)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"reply": reply})
}

// DELETE /history?user_id=xxx
func handleClearHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		userID = "default"
	}
	getSession(userID).clear()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})
}

// GET /health
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"model":   defaultModel,
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// loggingMiddleware logs every request
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// corsMiddleware adds CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	apiKey = os.Getenv("TOKENLAB_API_KEY")
	if apiKey == "" {
		log.Fatal("❌  TOKENLAB_API_KEY environment variable is required")
	}
	systemPrompt = os.Getenv("SYSTEM_PROMPT")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/chat", handleChat)
	mux.HandleFunc("/chat/simple", handleSimpleChat)
	mux.HandleFunc("/history", handleClearHistory)
	mux.HandleFunc("/health", handleHealth)

	handler := loggingMiddleware(corsMiddleware(mux))

	log.Printf("🚀  Server running on :%s | model: %s", port, defaultModel)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
