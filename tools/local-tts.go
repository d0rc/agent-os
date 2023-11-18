package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

var ttsQueue = make(chan string, 100000)

func init() {
	go func() {
		cache := make(map[string]bool)
		for {
			text := <-ttsQueue
			if _, ok := cache[text]; ok {
				continue
			}
			cache[text] = true
			_runLocalTTS(text)
		}
	}()
}

func RunLocalTTS(text string) {
	ttsQueue <- text
}

func _runLocalTTS(text string) {
	type ttsRequest struct {
		Text string `json:"text"`
	}
	ttsEndpoint := "http://localhost:5000/play"

	// use standard library to make a request
	requestJson, err := json.Marshal(ttsRequest{Text: text})
	_, err = http.Post(ttsEndpoint, "application/json", bytes.NewBuffer(requestJson))
	if err != nil {
		fmt.Println("failed to make a request to TTS server:", err)
		return
	}
}
