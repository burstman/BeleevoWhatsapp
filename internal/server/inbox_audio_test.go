package server

import "testing"

func TestCanonicalSendAudioMime(t *testing.T) {
	cases := map[string]string{
		"audio/ogg; codecs=opus": "audio/ogg",
		"audio/ogg":              "audio/ogg",
		"audio/opus":             "audio/ogg",
		"audio/mp4":              "audio/mp4",
		"audio/m4a":              "audio/mp4",
		"audio/x-m4a":            "audio/mp4",
		"audio/mpeg":             "audio/mpeg",
		"audio/mp3":              "audio/mpeg",
		"audio/mpga":             "audio/mpeg",
		"audio/aac":              "audio/aac",
		"audio/amr":              "audio/amr",
		"audio/amr-nb":           "audio/amr",
		"audio/x-amr":            "audio/amr",
		"audio/webm":             "",
		"video/webm":             "",
		"audio/x-wav":            "",
		"audio/x-flac":           "",
		"":                       "",
	}
	for in, want := range cases {
		if got := canonicalSendAudioMime(in); got != want {
			t.Errorf("canonicalSendAudioMime(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAudioFilename(t *testing.T) {
	if got := audioFilename("", "audio/ogg"); got != "voice.ogg" {
		t.Errorf("empty original with ogg = %q, want voice.ogg", got)
	}
	if got := audioFilename("my note.mp3", "audio/mpeg"); got != "my_note.mp3" {
		t.Errorf("kept safe original = %q, want my_note.mp3", got)
	}
	if got := audioFilename("../evil\x00name.flac", "audio/mp4"); got == "" || len(got) > 80 {
		t.Errorf("sanitized name should stay short and clean, got %q", got)
	}
}
