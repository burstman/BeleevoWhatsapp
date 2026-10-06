package whatsapp

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"testing"
)

// A microphone recording produced by Chrome: OPUS in an MP4 container, which
// Meta uploads but refuses to deliver. normalizeAudio must remux it into OGG.
func TestNormalizeAudioRemuxesOpusInMp4(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}

	opusMp4 := buildOpusMp4(t)
	s := &Service{}
	mime, out, err := s.normalizeAudio(context.Background(), "audio/mp4", opusMp4)
	if err != nil {
		t.Fatalf("normalizeAudio: %v", err)
	}
	if mime != "audio/ogg" {
		t.Errorf("mime = %q, want audio/ogg", mime)
	}
	if !bytes.HasPrefix(out, []byte("OggS")) {
		t.Error("remuxed bytes are not an OGG container")
	}
	if len(out) == 0 {
		t.Error("remux produced no bytes")
	}
}

// AAC-in-MP4 (Safari) is a standard M4A and must pass through untouched, since
// it is already the delivered format.
func TestNormalizeAudioKeepsSupportedFormats(t *testing.T) {
	s := &Service{}
	for _, in := range [][]byte{
		[]byte("OggS fake opus"),
		[]byte("ID3 fake mp3"),
		[]byte("#!AMR fake"),
	} {
		mime, out, err := s.normalizeAudio(context.Background(), "audio/ogg", in)
		if err != nil {
			t.Fatalf("normalizeAudio(o%x): %v", in[0], err)
		}
		if mime != "audio/ogg" || string(out) != string(in) {
			t.Errorf("ogg must pass through untouched, got %q", mime)
		}
	}
}

func buildOpusMp4(t *testing.T) []byte {
	t.Helper()
	tmp, err := exec.Command("mktemp", "-d").Output()
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	dir := string(bytes.TrimSpace(tmp))
	in := dir + "/in.mp4"
	out, err := exec.Command("ffmpeg", "-y", "-loglevel", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:a", "libopus", "-f", "mp4", in).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg fixture: %v\n%s", err, out)
	}
	defer exec.Command("rm", "-rf", dir).Run()
	data, err := os.ReadFile(in)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}
