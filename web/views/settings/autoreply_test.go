package settings

import (
	"strings"
	"testing"

	"whatsappconverty/internal/whatsapp"
	"whatsappconverty/web/views/components"
)

// The greeting page must render for each kind and carry the form fields the
// save handler reads back, otherwise a save silently posts nothing.
func TestAutoreplyPageRendersFields(t *testing.T) {
	cases := []struct {
		name string
		cfg  whatsapp.FirstContactAutoreply
		want []string
	}{
		{
			name: "text",
			cfg:  whatsapp.FirstContactAutoreply{Kind: whatsapp.AutoreplyText, Enabled: true, TextBody: "Bonjour !"},
			want: []string{`name="kind"`, `value="text"`, `name="text_body"`, "Bonjour !"},
		},
		{
			name: "image",
			cfg:  whatsapp.FirstContactAutoreply{Kind: whatsapp.AutoreplyImage, Enabled: true, Caption: "Voici", MediaMime: "image/png"},
			want: []string{`value="image"`, `name="image"`, `name="caption"`, "/settings/autoreply/media"},
		},
		{
			name: "audio",
			cfg:  whatsapp.FirstContactAutoreply{Kind: whatsapp.AutoreplyAudio, Enabled: true, MediaMime: "audio/ogg"},
			want: []string{`value="audio"`, `name="audio"`, "/settings/autoreply/media"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sb strings.Builder
			page := components.Page{Title: "Auto-reply", Active: "autoreply"}
			if err := Autoreply(page, page.I18N, tc.cfg, AutoreplyFlash{}).Render(t.Context(), &sb); err != nil {
				t.Fatalf("render: %v", err)
			}
			html := sb.String()
			for _, want := range tc.want {
				if !strings.Contains(html, want) {
					t.Errorf("rendered page missing %q", want)
				}
			}
			// The form must post multipart so the media kinds can carry a file.
			if !strings.Contains(html, `enctype="multipart/form-data"`) {
				t.Error("form must be multipart to carry an upload")
			}
		})
	}
}
