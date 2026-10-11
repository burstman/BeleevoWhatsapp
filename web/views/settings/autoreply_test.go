package settings

import (
	"strings"
	"testing"

	"whatsappconverty/internal/i18n"
	"whatsappconverty/internal/whatsapp"
)

// The modal fragment is the only thing /settings/autoreply renders: the inbox
// swaps it into #autoreply-modal-body. It must carry every field the save
// handler reads and post multipart so the media kinds can carry a file.
func TestAutoreplyModalRender(t *testing.T) {
	cases := []struct {
		name string
		cfg  whatsapp.FirstContactAutoreply
		want []string
	}{
		{
			name: "text",
			cfg:  whatsapp.FirstContactAutoreply{Kind: "text", Enabled: true, TextBody: "Bonjour"},
			want: []string{`name="kind"`, `value="text"`, `name="text_body"`, "Bonjour"},
		},
		{
			name: "image",
			cfg:  whatsapp.FirstContactAutoreply{Kind: "image", Enabled: true, Caption: "Legende", MediaMime: "image/jpeg"},
			want: []string{`value="image"`, `name="image"`, `name="caption"`, "/settings/autoreply/media"},
		},
		{
			name: "audio",
			cfg:  whatsapp.FirstContactAutoreply{Kind: "audio", Enabled: true, MediaMime: "audio/ogg"},
			want: []string{`value="audio"`, `name="audio"`, "/settings/autoreply/media"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sb strings.Builder
			if err := AutoreplyModal(i18n.New(i18n.En), tc.cfg, AutoreplyFlash{}).Render(t.Context(), &sb); err != nil {
				t.Fatalf("render: %v", err)
			}
			html := sb.String()
			for _, want := range tc.want {
				if !strings.Contains(html, want) {
					t.Errorf("rendered modal missing %q", want)
				}
			}
			if !strings.Contains(html, `hx-post="/settings/autoreply"`) {
				t.Error("form must post to the auto-reply endpoint")
			}
			if !strings.Contains(html, `hx-encoding="multipart/form-data"`) {
				t.Error("form must be multipart to carry an upload")
			}
			if !strings.Contains(html, `hx-target="#autoreply-modal-body"`) {
				t.Error("form must swap back into the modal body")
			}
		})
	}
}

// The "Enabled" control is a sliding switch: it must keep the native
// checkbox field the save handler reads, reflect the saved state, and carry
// the peer classes that draw the toggle.
func TestAutoreplyToggle(t *testing.T) {
	render := func(enabled bool) string {
		var sb strings.Builder
		cfg := whatsapp.FirstContactAutoreply{Kind: "text", Enabled: enabled, TextBody: "Bonjour"}
		if err := AutoreplyModal(i18n.New(i18n.En), cfg, AutoreplyFlash{}).Render(t.Context(), &sb); err != nil {
			t.Fatalf("render: %v", err)
		}
		return sb.String()
	}

	on := render(true)
	if !strings.Contains(on, `name="enabled"`) {
		t.Error("enabled field missing")
	}
	if !strings.Contains(on, `role="switch"`) {
		t.Error("enabled control must be a switch")
	}
	if !strings.Contains(on, `peer-checked:bg-indigo-600`) {
		t.Error("toggle track missing its checked style")
	}
	if !strings.Contains(on, `value="1" checked`) {
		t.Error("enabled toggle must render checked when enabled")
	}

	off := render(false)
	if strings.Contains(off, `value="1" checked`) {
		t.Error("enabled toggle must not render checked when disabled")
	}
}

// A save problem must surface as a visible banner inside the modal.
func TestAutoreplyModalFlash(t *testing.T) {
	var sb strings.Builder
	flash := AutoreplyFlash{Error: "missing file"}
	if err := AutoreplyModal(i18n.New(i18n.En), whatsapp.FirstContactAutoreply{Kind: "image"}, flash).Render(t.Context(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(sb.String(), "missing file") {
		t.Error("error flash not rendered")
	}
}
