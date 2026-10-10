package viewsdashboard

import (
	"encoding/json"

	"whatsappconverty/internal/i18n"
)

// automationFormMessages returns the client-side validation strings as a JSON
// object the form script reads under window.automationFormI18N. The strings are
// injected from the dict so the inline validation matches the page language
// without shipping a parallel dictionary in the script. %s placeholders are
// filled by the script with the runtime values (timezone, clock, send time).
func automationFormMessages(dict *i18n.Dict) string {
	b, err := json.Marshal(map[string]string{
		"clockNow":        dict.T("form.clockNow"),
		"clockUnreadable": dict.T("form.clockUnreadable"),
		"noTemplate":      dict.T("form.noTemplate"),
		"preview":         dict.T("form.previewEmpty"),
		"chooseTemplate":  dict.T("form.chooseTemplate"),
		"needTime":        dict.T("form.needTime"),
		"needDay":         dict.T("form.needDay"),
		"needTz":          dict.T("form.needTz"),
		"needDelay":       dict.T("form.needDelay"),
	})
	if err != nil {
		return "{}"
	}
	return string(b)
}