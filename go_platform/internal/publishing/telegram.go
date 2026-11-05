package publishing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

type DeliveryError struct {
	Uncertain   bool
	Retryable   bool
	Status      int
	Description string
}

func (e *DeliveryError) Error() string { return e.Description }

type TelegramSender struct {
	client   *http.Client
	endpoint string
}

func NewTelegramSender(token string) (*TelegramSender, error) {
	if token == "" || strings.ContainsAny(token, "/\\?&#\r\n ") {
		return nil, errors.New("invalid Telegram token")
	}
	return &TelegramSender{
		client:   &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		endpoint: "https://api.telegram.org/bot" + token + "/sendPhoto",
	}, nil
}

func (s *TelegramSender) SendPhoto(ctx context.Context, chatID, photoPath, caption string) (int64, error) {
	if chatID == "" || utf8.RuneCountInString(caption) > PhotoCaptionLimit {
		return 0, errors.New("invalid Telegram destination or caption")
	}
	file, err := os.Open(photoPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return 0, errors.New("photo file is empty or not regular")
	}
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		if err := writer.WriteField("chat_id", chatID); err != nil {
			pw.CloseWithError(err)
			return
		}
		if err := writer.WriteField("caption", caption); err != nil {
			pw.CloseWithError(err)
			return
		}
		part, err := writer.CreateFormFile("photo", filepath.Base(photoPath))
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		if _, err = io.Copy(part, file); err != nil {
			pw.CloseWithError(err)
			return
		}
		if err = writer.Close(); err != nil {
			pw.CloseWithError(err)
			return
		}
		pw.Close()
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, pr)
	if err != nil {
		pr.Close()
		return 0, errors.New("could not create Telegram request")
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := s.client.Do(req)
	pr.Close()
	if err != nil {
		return 0, &DeliveryError{Uncertain: true, Description: "Telegram response was not received"}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16_385))
	if err != nil || len(body) > 16_384 {
		return 0, &DeliveryError{Uncertain: true, Status: resp.StatusCode, Description: "Telegram response could not be read"}
	}
	var parsed struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return 0, &DeliveryError{Uncertain: true, Status: resp.StatusCode, Description: "Telegram returned an invalid response"}
	}
	if resp.StatusCode == 200 && parsed.OK && parsed.Result.MessageID > 0 {
		return parsed.Result.MessageID, nil
	}
	if resp.StatusCode == 429 {
		return 0, &DeliveryError{Retryable: true, Status: 429, Description: "Telegram rate limited the photo"}
	}
	if resp.StatusCode >= 500 {
		return 0, &DeliveryError{Uncertain: true, Status: resp.StatusCode, Description: "Telegram server response is uncertain"}
	}
	description := strings.TrimSpace(parsed.Description)
	if description == "" {
		description = fmt.Sprintf("Telegram rejected photo with status %d", resp.StatusCode)
	}
	return 0, &DeliveryError{Status: resp.StatusCode, Description: description}
}
