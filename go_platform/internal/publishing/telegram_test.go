package publishing

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func testPhoto(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evidence.png")
	if err := os.WriteFile(path, []byte("PNG test bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTelegramSendsPhotoAndReadsMessageID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method: %s", r.Method)
		}
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Error(err)
			return
		}
		if r.FormValue("chat_id") != "@test_channel" || r.FormValue("caption") != "عرض مؤكد" {
			t.Errorf("wrong fields")
		}
		file, _, err := r.FormFile("photo")
		if err != nil {
			t.Error(err)
			return
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		if string(data) != "PNG test bytes" {
			t.Errorf("wrong photo bytes")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true,"result":{"message_id":123}}`)
	}))
	defer server.Close()
	sender, _ := NewTelegramSender("secret")
	sender.endpoint = server.URL
	id, err := sender.SendPhoto(context.Background(), "@test_channel", testPhoto(t), "عرض مؤكد")
	if err != nil || id != 123 {
		t.Fatalf("id=%d err=%v", id, err)
	}
}

func TestTelegramDistinguishesKnownRejectionAndUncertainResponse(t *testing.T) {
	for _, tc := range []struct {
		status               int
		body                 string
		retryable, uncertain bool
	}{
		{429, `{"ok":false,"description":"Too Many Requests"}`, true, false},
		{500, `{"ok":false,"description":"Internal Server Error"}`, false, true},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.Copy(io.Discard, r.Body)
			w.WriteHeader(tc.status)
			io.WriteString(w, tc.body)
		}))
		sender, _ := NewTelegramSender("secret")
		sender.endpoint = server.URL
		_, err := sender.SendPhoto(context.Background(), "@test_channel", testPhoto(t), "عرض مؤكد")
		var delivery *DeliveryError
		if !errors.As(err, &delivery) || delivery.Retryable != tc.retryable || delivery.Uncertain != tc.uncertain {
			t.Fatalf("status=%d err=%v", tc.status, err)
		}
		server.Close()
	}
}
