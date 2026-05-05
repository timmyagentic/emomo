package service

import "testing"

func TestSanitizeInvalidJSONEscapesPreservesLegalEscapes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"unchanged plain", `hello world`, `hello world`},
		{"unchanged json with legal escapes",
			`{"a":"line1\nline2\t\"quoted\""}`,
			`{"a":"line1\nline2\t\"quoted\""}`},
		{"unchanged unicode escape",
			`{"a":"\u4e2d\u6587"}`,
			`{"a":"\u4e2d\u6587"}`},
		{"strip orphan backslash before space",
			`{"a":"foo\ |bar"}`,
			`{"a":"foo |bar"}`},
		{"strip orphan backslash before pipe",
			`{"a":"foo\|bar"}`,
			`{"a":"foo|bar"}`},
		{"strip orphan backslash before chinese rune (multi-byte)",
			`{"a":"foo\中bar"}`,
			`{"a":"foo中bar"}`},
		{"strip multiple orphan backslashes mixed with legal escapes",
			`{"ocr_text":"line1\ |line2\nline3\ |line4"}`,
			`{"ocr_text":"line1 |line2\nline3 |line4"}`},
		{"strip trailing standalone backslash",
			`{"a":"foo\`,
			`{"a":"foo`},
		{"strip invalid \\u sequence (less than 4 hex)",
			`{"a":"\u12gh"}`,
			`{"a":"u12gh"}`},
		{"keep \\\\ as literal escaped backslash",
			`{"a":"foo\\bar"}`,
			`{"a":"foo\\bar"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeInvalidJSONEscapes(tc.in)
			if got != tc.want {
				t.Fatalf("sanitizeInvalidJSONEscapes(%q)\n  got  %q\n  want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseVLMAnalysisRecoversFromOrphanBackslashEscapes(t *testing.T) {
	t.Parallel()

	// Real-shape sample drawn from the production failure logs: GLM-4.6V
	// emitted `\ ` between OCR run separators, which `json.Unmarshal` rejects.
	raw := `{"ocr_text":"你将会毕业 | 你将会拿到那个学位\ | 你将会离开这个该死的学校\ | 谢了 人类","description":"画面主体是戴学士帽的立式镜子（拟人化形象）。"}`

	got, ok := parseVLMAnalysis(raw)
	if !ok {
		t.Fatalf("parseVLMAnalysis() ok = false, want true")
	}
	if got.OCRText == "" || got.Description == "" {
		t.Fatalf("parseVLMAnalysis() empty fields: %+v", got)
	}
	// The orphan backslash should have been dropped, leaving the OCR text
	// readable and stable.
	if want := "你将会毕业 | 你将会拿到那个学位 | 你将会离开这个该死的学校 | 谢了 人类"; got.OCRText != want {
		t.Fatalf("OCRText\n  got  %q\n  want %q", got.OCRText, want)
	}
	if want := "画面主体是戴学士帽的立式镜子（拟人化形象）。"; got.Description != want {
		t.Fatalf("Description\n  got  %q\n  want %q", got.Description, want)
	}
}

func TestParseVLMAnalysisStillRejectsRawTextOnly(t *testing.T) {
	t.Parallel()

	// Pure prose — no JSON structure at all — must remain a hard failure.
	got, ok := parseVLMAnalysis(`这只是一段说明，模型没产出 JSON。`)
	if ok {
		t.Fatalf("parseVLMAnalysis(prose) ok = true, want false")
	}
	if got == nil {
		t.Fatal("parseVLMAnalysis(prose) returned nil analysis; want non-nil with raw description")
	}
}

func TestParseVLMAnalysisStripsCodeFenceBeforeSanitize(t *testing.T) {
	t.Parallel()

	// Combine markdown fence (which the prompt explicitly forbids) with the
	// orphan-backslash bug — both fallbacks need to compose.
	raw := "```json\n" +
		`{"ocr_text":"foo\ |bar","description":"hello"}` +
		"\n```"

	got, ok := parseVLMAnalysis(raw)
	if !ok {
		t.Fatalf("parseVLMAnalysis() ok = false, want true")
	}
	if got.OCRText != "foo |bar" {
		t.Fatalf("OCRText = %q, want %q", got.OCRText, "foo |bar")
	}
	if got.Description != "hello" {
		t.Fatalf("Description = %q, want %q", got.Description, "hello")
	}
}
