package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

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

func callAPI(messages []Message, model string, stream bool) (*http.Response, error) {
	payload := ChatRequest{
		Model:       model,
		Messages:    messages,
		Stream:      stream,
		Temperature: 0.2, // গভীর ও সঠিক রেজনির জন্য টেম্পারেচার স্ট্যান্ডার্ড রাখা হলো
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
	if req.UserID == "" { req.UserID = "default" }
	if req.Message == "" {
		http.Error(w, `{"error":"message is required"}`, http.StatusBadRequest)
		return
	}
	if req.Model == "" { req.Model = defaultModel }

	if req.Message == "/clear" {
		getSession(req.UserID).clear()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"reply": "✅ History cleared!"})
		return
	}

	session := getSession(req.UserID)
	messages := session.getMessages(req.Message)

	var finalMessages []Message
	if systemPrompt != "" {
		finalMessages = append(finalMessages, Message{Role: "system", Content: systemPrompt})
	}

	if ShouldSearchQuery(req.Message) {
		log.Printf("🔍 [Tavily Search] active for query: %s", req.Message)
		searchResult := SearchInternet(req.Message)
		if searchResult != "" {
			finalMessages = append(finalMessages, Message{Role: "system", Content: searchResult})
		}
	}

	finalMessages = append(finalMessages, messages...)

	resp, err := callAPI(finalMessages, req.Model, true)
	if err != nil {
		log.Printf("API error: %v", err)
		http.Error(w, `{"error":"upstream API error"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, canFlush := w.(http.Flusher)

	var fullReply strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") { continue }
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" { break }

		var chunk ChatResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil { continue }
		if chunk.Error != nil {
			fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", chunk.Error.Message)
			if canFlush { flusher.Flush() }
			return
		}

		for _, choice := range chunk.Choices {
			if choice.Delta != nil && choice.Delta.Content != "" {
				token := choice.Delta.Content
				fullReply.WriteString(token)
				b, _ := json.Marshal(token)
				fmt.Fprintf(w, "data: %s\n\n", string(b))
				if canFlush { flusher.Flush() }
			}
		}
	}

	if fullReply.Len() > 0 {
		session.addMessage("user", req.Message)
		session.addMessage("assistant", fullReply.String())
	}

	fmt.Fprint(w, "data: [DONE]\n\n")
	if canFlush { flusher.Flush() }
}

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
	if req.UserID == "" { req.UserID = "default" }
	if req.Message == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "message is required"})
		return
	}
	if req.Model == "" { req.Model = defaultModel }

	session := getSession(req.UserID)
	messages := session.getMessages(req.Message)

	var finalMessages []Message
	if systemPrompt != "" {
		finalMessages = append(finalMessages, Message{Role: "system", Content: systemPrompt})
	}

	if ShouldSearchQuery(req.Message) {
		log.Printf("🔍 [Tavily Search Simple] active for query: %s", req.Message)
		searchResult := SearchInternet(req.Message)
		if searchResult != "" {
			finalMessages = append(finalMessages, Message{Role: "system", Content: searchResult})
		}
	}

	finalMessages = append(finalMessages, messages...)

	resp, err := callAPI(finalMessages, req.Model, false)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": "upstream API error"})
		return
	}
	defer resp.Body.Close()

	var result ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to parse API response"})
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

func handleClearHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	userID := r.URL.Query().Get("user_id")
	if userID == "" { userID = "default" }
	getSession(userID).clear()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"model":  defaultModel,
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}
