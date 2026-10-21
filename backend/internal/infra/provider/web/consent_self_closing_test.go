package web

import "testing"

// TestExtractConsentTokenSelfClosingInput 覆盖表单隐藏字段的自闭合写法。
func TestExtractConsentTokenSelfClosingInput(t *testing.T) {
	const want = "eyJhbGciOiJFUzI1NiJ9.eyJkYyI6ImRjIn0.sig"
	for _, input := range []string{
		`<input type="hidden" name="consent_token" value="` + want + `"/>`,
		`<input value="` + want + `" name="consent_token" type="hidden" />`,
	} {
		if got := extractConsentToken([]byte(input)); got != want {
			t.Errorf("extractConsentToken(%q) = %q, want %q", input, got, want)
		}
	}
}
