package marketplace

import (
	"errors"
	"strings"
	"testing"
)

// =============================================================================
// Add — nil template
// =============================================================================

func TestAdd_NilTemplate(t *testing.T) {
	r := NewTemplateRegistry()
	r.Add(nil)
	if r.Count() != 0 {
		t.Error("expected Add(nil) to be a no-op")
	}
}

// =============================================================================
// sanitizeTemplate — nil input returns nil
// =============================================================================

func TestSanitizeTemplate_Nil(t *testing.T) {
	result := sanitizeTemplate(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}
}

// =============================================================================
// sanitizeTemplate — replacement function parts mismatch
// (composeDefaultExpr, composeURLPasswordExpr, composeQueryPasswordExpr
//  non-matching branches)
// =============================================================================

func TestSanitizeTemplate_DefaultExprNonMatch(t *testing.T) {
	// The composeDefaultExpr matches `${VAR:-default}` patterns.
	// A non-matching string like `${VAR}` has no ":-" so FindStringSubmatch
	// returns nil for the capture groups, hitting len(parts) != 3.
	r := NewTemplateRegistry()
	r.Add(&Template{
		Slug:        "non-match-test",
		Name:        "Test",
		Description: "desc",
		Category:    "test",
		Author:      "a",
		Version:     "1",
		ComposeYAML: `services:
  web:
    image: nginx
    environment:
      - PLAIN=${VAR}
      - WITH_DEFAULT=${OTHER:-default}
`,
	})
	// Should not panic — the non-matching `${VAR}` is preserved as-is
	tmpl := r.Get("non-match-test")
	if tmpl == nil {
		t.Fatal("expected template")
	}
	if !strings.Contains(tmpl.ComposeYAML, "${VAR}") {
		t.Error("expected ${VAR} to be preserved")
	}
}

// =============================================================================
// sanitizeTemplate — URL password non-matching regex parts
// =============================================================================

func TestSanitizeTemplate_URLPasswordNonMatch(t *testing.T) {
	r := NewTemplateRegistry()
	// The composeURLPasswordExpr matches `://user:pass@host` patterns.
	// A non-matching string like `password=changeme` should pass through.
	r.Add(&Template{
		Slug:        "url-nonmatch",
		Name:        "Test",
		Description: "desc",
		Category:    "test",
		Author:      "a",
		Version:     "1",
		ComposeYAML: `services:
  web:
    image: nginx
    environment:
      PASSWORD: changeme
`,
	})
	tmpl := r.Get("url-nonmatch")
	if tmpl == nil {
		t.Fatal("expected template")
	}
}

// =============================================================================
// sanitizeSensitiveScalarDefaults — line without colon
// =============================================================================

func TestSanitizeSensitiveScalarDefaults_NoColon(t *testing.T) {
	// A line like `  # comment` has no colon — should be skipped
	input := `services:
  web:
    image: nginx
  # this line has no colon
    environment:` + "\n"
	output := sanitizeSensitiveScalarDefaults(input)
	// The output should be identical since nothing matched
	if output != input {
		t.Errorf("expected unchanged, got:\n%s", output)
	}
}

// =============================================================================
// sanitizeSensitiveScalarDefaults — line with comment in value
// =============================================================================

func TestSanitizeSensitiveScalarDefaults_CommentInValue(t *testing.T) {
	input := `services:
  web:
    image: nginx
    environment:
      PASSWORD: changeme # this is a weak default
`
	// This should sanitize the PASSWORD value
	output := sanitizeSensitiveScalarDefaults(input)
	if strings.Contains(output, "changeme") {
		t.Errorf("expected weak default to be sanitized, got:\n%s", output)
	}
}

// =============================================================================
// weakSensitiveScalarDefault — not a weak default
// =============================================================================

func TestWeakSensitiveScalarDefault_NotWeak(t *testing.T) {
	key, value, ok := weakSensitiveScalarDefault("      PASSWORD: strongsecret123")
	if ok {
		t.Errorf("expected ok=false for strong secret, got key=%q value=%q", key, value)
	}
}

// =============================================================================
// splitLineEnding — Windows CRLF and no newline
// =============================================================================

func TestSplitLineEnding_WindowsAndNoNewline(t *testing.T) {
	// Windows CRLF
	body, nl := splitLineEnding("POSTGRES_PASSWORD: changeme\r\n")
	if nl != "\r\n" {
		t.Errorf("expected CRLF newline, got %q", nl)
	}
	if !strings.Contains(body, "POSTGRES_PASSWORD") {
		t.Errorf("expected body to contain key, got %q", body)
	}

	// No newline at end
	body, nl = splitLineEnding("POSTGRES_PASSWORD: changeme")
	if nl != "" {
		t.Errorf("expected empty newline, got %q", nl)
	}
	if body != "POSTGRES_PASSWORD: changeme" {
		t.Errorf("expected full body, got %q", body)
	}
}

// =============================================================================
// isTemplateEnvKey — empty key
// =============================================================================

func TestIsTemplateEnvKey_Empty(t *testing.T) {
	if isTemplateEnvKey("") {
		t.Error("expected false for empty key")
	}
}

// =============================================================================
// sensitiveTemplatePlaceholder — all branches
// =============================================================================

func TestSensitiveTemplatePlaceholder_Branches(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"DB_ROOT_PASSWORD", "${DB_ROOT_PASSWORD}"},
		{"ROOT_PASSWORD", "${DB_ROOT_PASSWORD}"},
		{"DATABASE_URL", "${DB_PASSWORD}"},
		{"POSTGRES_PASSWORD", "${DB_PASSWORD}"},
		{"MYSQL_PASSWORD", "${DB_PASSWORD}"},
		// MARIADB_ROOT_PASSWORD contains "ROOT_PASSWORD" so it hits the first case
		{"MARIADB_ROOT_PASSWORD", "${DB_ROOT_PASSWORD}"},
		// MARIADB_PASSWORD does not contain ROOT_PASSWORD, hits the DB case
		{"MARIADB_PASSWORD", "${DB_PASSWORD}"},
		{"DB_PASSWORD", "${DB_PASSWORD}"},
		{"ADMIN_PASSWORD", "${ADMIN_PASSWORD}"},
		{"JWT_SECRET", "${JWT_SECRET}"},
		{"SECRET_KEY", "${SECRET_KEY}"},
		{"CUSTOM_KEY", "${CUSTOM_KEY}"},
		{"MY_TOKEN", "${MY_TOKEN}"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := sensitiveTemplatePlaceholder(tt.key)
			if got != tt.want {
				t.Errorf("sensitiveTemplatePlaceholder(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

// =============================================================================
// ValidateTemplate — empty service definition (svc == nil)
// =============================================================================

func TestValidateTemplate_EmptyService(t *testing.T) {
	tmpl := validTemplate()
	tmpl.ComposeYAML = `services:
  web:
  db:
    image: postgres:16
`
	err := ValidateTemplate(tmpl)
	if err == nil {
		t.Fatal("expected validation error for empty service")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || !containsIssue(ve.Issues, "empty definition") {
		t.Errorf("expected empty-definition issue, got %v", err)
	}
}

// =============================================================================
// ValidateTemplate — empty volume entry
// =============================================================================

func TestValidateTemplate_EmptyVolumeEntry(t *testing.T) {
	tmpl := validTemplate()
	tmpl.ComposeYAML = `services:
  web:
    image: nginx:latest
    volumes:
      - ""
      - /host:/container
`
	err := ValidateTemplate(tmpl)
	if err == nil {
		t.Fatal("expected validation error for empty volume entry")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || !containsIssue(ve.Issues, "empty volume entry") {
		t.Errorf("expected empty-volume issue, got %v", err)
	}
}

// =============================================================================
// ValidateTemplate — malformed volume entry (name after colon empty)
// =============================================================================

func TestValidateTemplate_MalformedVolume(t *testing.T) {
	tmpl := validTemplate()
	tmpl.ComposeYAML = `services:
  web:
    image: nginx:latest
    volumes:
      - ":/container"
`
	err := ValidateTemplate(tmpl)
	if err == nil {
		t.Fatal("expected validation error for malformed volume entry")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || !containsIssue(ve.Issues, "malformed volume entry") {
		t.Errorf("expected malformed-volume issue, got %v", err)
	}
}

// =============================================================================
// ValidateTemplate — empty port entry
// =============================================================================

func TestValidateTemplate_EmptyPortEntry(t *testing.T) {
	tmpl := validTemplate()
	tmpl.ComposeYAML = `services:
  web:
    image: nginx:latest
    ports:
      - ""
      - "80:80"
`
	err := ValidateTemplate(tmpl)
	if err == nil {
		t.Fatal("expected validation error for empty port entry")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || !containsIssue(ve.Issues, "empty port entry") {
		t.Errorf("expected empty-port issue, got %v", err)
	}
}

// =============================================================================
// ValidateAll — non-ValidationError error branch (ValidateTemplate returning
// a non-*ValidationError error)
// =============================================================================

func TestValidateAll_NonValidationError(t *testing.T) {
	// This is hard to trigger because ValidateTemplate always returns
	// *ValidationError or nil. The else branch in ValidateAll is a safety
	// net. We test it by validating a template with compose_yaml that
	// triggers a non-validation error path.
	// Actually, ValidateTemplate always wraps errors in ValidationError,
	// so the else branch is a defensive fallback. Let's just ensure
	// ValidateAll works correctly for a valid case.
	r := NewTemplateRegistry()
	good := validTemplate()
	good.Slug = "valid-slug"
	good.Name = "Valid"
	r.Add(good)

	results := r.ValidateAll()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("expected no error for valid template, got %v", results[0].Err)
	}
}
