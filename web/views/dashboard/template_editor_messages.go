package viewsdashboard

import (
	"encoding/json"

	"whatsappconverty/internal/i18n"
)

// templateEditorMessages returns the template editor's client-side strings as
// a JSON object the editor script reads under window.templateEditorI18N, so
// the live Meta body rules match the page language. %s placeholders are filled
// by window.templateEditorFmt with the runtime values (token names, word counts).
func templateEditorMessages(dict *i18n.Dict) string {
	b, err := json.Marshal(map[string]string{
		"startVar":   dict.T("tpl.js.startVar"),
		"endVar":     dict.T("tpl.js.endVar"),
		"needsWords": dict.T("tpl.js.needsWords"),
		"vars":       dict.T("tpl.js.vars"),
		"exampleFor": dict.T("tpl.js.exampleFor"),
		"example":    dict.T("tpl.js.example"),
		"expiresIn":  dict.T("tpl.js.expiresIn"),
		"expired":    dict.T("tpl.js.expired"),
	})
	if err != nil {
		return "{}"
	}
	return string(b)
}
