package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"
	openai "github.com/sashabaranov/go-openai"
)

// EventDetails is the structured result parsed from a transcribed voice reminder.
type EventDetails struct {
	Time        string `json:"time"`
	Description string `json:"description"`
}

// handleVoiceMessage downloads a voice note and returns its transcription.
func (b *Bot) handleVoiceMessage(update tgbotapi.Update) (string, error) {
	file, err := b.bot.GetFile(tgbotapi.FileConfig{FileID: update.Message.Voice.FileID})
	if err != nil {
		log.Printf("Error getting file: %v\n", err)
		return "", fmt.Errorf("error getting file: %v", err)
	}

	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", botToken, file.FilePath)
	response, err := http.Get(fileURL)
	if err != nil {
		log.Printf("Error downloading file: %v\n", err)
		return "", fmt.Errorf("error downloading file: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		log.Printf("Error downloading file: unexpected status %d from %s\n", response.StatusCode, fileURL)
		return "", fmt.Errorf("unexpected status %d downloading voice file", response.StatusCode)
	}

	dir := fmt.Sprintf("voices/%v", update.Message.From.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("error creating directory: %v", err)
	}

	filePath := fmt.Sprintf("%s/voice_%d.ogg", dir, update.Message.MessageID)
	out, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("error creating file: %v", err)
	}
	defer os.Remove(filePath)

	if _, err := io.Copy(out, response.Body); err != nil {
		out.Close()
		log.Printf("error saving file: %v\n", err)
		return "", fmt.Errorf("error saving file: %v", err)
	}
	out.Close()
	log.Printf("Voice message saved: %v\n", filePath)

	text, err := transcribeAudioWithOpenAI(filePath)
	if err != nil {
		return "", fmt.Errorf("error transcribing file: %v", err)
	}
	return text, nil
}

func transcribeAudioWithOpenAI(filePath string) (string, error) {
	request := openai.AudioRequest{
		Model:       "whisper-1",
		FilePath:    filePath,
		Prompt:      "This is a command is for setting a reminder for a jira task.",
		Temperature: 0.2,
		Language:    "en",
		Format:      "text",
	}

	response, err := openai.NewClient(openaiToken).CreateTranscription(context.Background(), request)
	if err != nil {
		return "", err
	}
	log.Printf("Transcribed text: %v\n", response.Text)
	return response.Text, nil
}

func processTextWithOpenAI(transcription string) (string, error) {
	prompt := fmt.Sprintf(`Extract the following details from the text:
1. Time in this format (d for days ,h for hours m for min ) (e.g., "in three hourse => 3h").
2. Description (e.g., "team meeting").
3. Return the result like: (just this no json in the beginning)
{
  "time": "3h",
  "description": "reminder"
}


Text: "%s"`, transcription)

	req := openai.ChatCompletionRequest{
		Model: "gpt-4o-mini",
		Messages: []openai.ChatCompletionMessage{
			{Role: "system", Content: "You are a helpful assistant for parsing text into structured data."},
			{Role: "user", Content: prompt},
		},
		MaxTokens: 150,
	}

	resp, err := openai.NewClient(openaiToken).CreateChatCompletion(context.Background(), req)
	if err != nil {
		return "", fmt.Errorf("failed to process text with OpenAI: %w", err)
	}
	return resp.Choices[0].Message.Content, nil
}

func unmarshalResponse(response string) (*EventDetails, error) {
	log.Printf("Response: %v\n", response)
	var event EventDetails
	if err := json.Unmarshal([]byte(response), &event); err != nil {
		return nil, err
	}
	return &event, nil
}
