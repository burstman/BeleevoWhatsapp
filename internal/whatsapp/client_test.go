package whatsapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"whatsappconverty/internal/config"
)

// Meta decides the uploaded media's type from the multipart file part's
// Content-Type, not from the "type" form field. Go's CreateFormFile stamps
// application/octet-stream, which Meta rejects — pin the real mime here.
func TestUploadMediaSendsRealContentType(t *testing.T) {
	wantBody := []byte("OggS\x00 actual opus bytes")
	var gotCT, gotTypeField, gotFilename string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v21.0/12345/media" {
			t.Errorf("path = %q, want /v21.0/12345/media", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok-abc" {
			t.Errorf("Authorization = %q, want Bearer tok-abc", got)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("file part: %v", err)
		}
		gotCT = hdr.Header.Get("Content-Type")
		gotFilename = hdr.Filename
		gotBody, _ = io.ReadAll(f)
		gotTypeField = r.FormValue("type")
		_ = f.Close()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"media123"}`))
	}))
	defer srv.Close()

	s := &Service{cfg: config.Config{MetaGraphURL: srv.URL}, http: &http.Client{Timeout: 5 * time.Second}}
	id, err := s.UploadMedia(context.Background(), "tok-abc", "12345", "audio/ogg", wantBody, "voice.ogg")
	if err != nil {
		t.Fatalf("UploadMedia: %v", err)
	}
	if id != "media123" {
		t.Errorf("id = %q, want media123", id)
	}
	if gotCT != "audio/ogg" {
		t.Errorf("file part Content-Type = %q, want audio/ogg (Meta rejects octet-stream)", gotCT)
	}
	if gotTypeField != "audio/ogg" {
		t.Errorf("type field = %q, want audio/ogg", gotTypeField)
	}
	if string(gotBody) != string(wantBody) {
		t.Errorf("uploaded body mismatch")
	}
	if gotFilename != "voice.ogg" {
		t.Errorf("filename = %q, want voice.ogg", gotFilename)
	}
}

// A voice-note send must carry audio:{id, voice:true} and a plain attached
// audio must not.
func TestSendAudioPayload(t *testing.T) {
	gotID := make(chan string, 1)
	gotVoice := make(chan bool, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v21.0/12345/messages" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var payload struct {
			Type  string `json:"type"`
			Audio struct {
				ID    string `json:"id"`
				Voice *bool  `json:"voice"`
			} `json:"audio"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if payload.Type != "audio" {
			t.Errorf("type = %q, want audio", payload.Type)
		}
		gotID <- payload.Audio.ID
		voice := payload.Audio.Voice != nil && *payload.Audio.Voice
		gotVoice <- voice
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.meta"}]}`))
	}))
	defer srv.Close()

	s := &Service{cfg: config.Config{MetaGraphURL: srv.URL}, http: &http.Client{Timeout: 5 * time.Second}}

	id, err := s.SendAudio(context.Background(), "tok", "12345", "21654116584", "media123", true)
	if err != nil {
		t.Fatalf("SendAudio: %v", err)
	}
	if id != "wamid.meta" {
		t.Errorf("meta id = %q, want wamid.meta", id)
	}
	if <-gotID != "media123" || !<-gotVoice {
		t.Error("voice-note payload must reference media123 with voice:true")
	}

	if _, err := s.SendAudio(context.Background(), "tok", "12345", "21654116584", "media456", false); err != nil {
		t.Fatalf("SendAudio plain: %v", err)
	}
	if <-gotID != "media456" || <-gotVoice {
		t.Error("plain audio payload must not set voice")
	}
}

// An image send must carry type:"image" with image:{id} and the caption.
func TestSendImagePayload(t *testing.T) {
	got := make(chan string, 1)
	gotCaption := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v21.0/12345/messages" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var payload struct {
			Type  string `json:"type"`
			Image struct {
				ID      string `json:"id"`
				Caption string `json:"caption"`
			} `json:"image"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if payload.Type != "image" {
			t.Errorf("type = %q, want image", payload.Type)
		}
		got <- payload.Image.ID
		gotCaption <- payload.Image.Caption
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.img"}]}`))
	}))
	defer srv.Close()

	s := &Service{cfg: config.Config{MetaGraphURL: srv.URL}, http: &http.Client{Timeout: 5 * time.Second}}

	id, err := s.SendImage(context.Background(), "tok", "12345", "21654116584", "img123", "voici votre commande")
	if err != nil {
		t.Fatalf("SendImage: %v", err)
	}
	if id != "wamid.img" {
		t.Errorf("meta id = %q, want wamid.img", id)
	}
	if <-got != "img123" {
		t.Error("image payload must reference img123")
	}
	if cap := <-gotCaption; cap != "voici votre commande" {
		t.Errorf("caption = %q, want the passed caption", cap)
	}
}

// The multipart boundary must contain only characters a form post tolerates.
func TestUploadMediaBoundaryIsClean(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "multipart/form-data; boundary=") {
			t.Errorf("Content-Type = %q", ct)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m1"}`))
	}))
	defer srv.Close()

	s := &Service{cfg: config.Config{MetaGraphURL: srv.URL}, http: &http.Client{Timeout: 5 * time.Second}}
	if _, err := s.UploadMedia(context.Background(), "tok", "12345", "audio/mp4", []byte("ftyp"), "voice.m4a"); err != nil {
		t.Fatalf("UploadMedia: %v", err)
	}
}
