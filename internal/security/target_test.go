package security

import "testing"

func TestValidateTargetURL(t *testing.T) {
	allowed := []string{
		"https://example.com",
		"https://example.com/path?q=1",
		"http://93.184.216.34/",
	}
	for _, raw := range allowed {
		if _, err := ValidateTargetURL(raw, false); err != nil {
			t.Fatalf("%q should be allowed: %v", raw, err)
		}
	}

	blocked := []string{
		"https://localhost/",
		"http://127.0.0.1/",
		"http://10.0.0.5/",
		"http://192.168.1.1/",
		"http://169.254.169.254/latest/meta-data",
		"http://[::1]/",
		"http://metadata.google.internal/",
		"ftp://example.com/",
		"https://app.localhost/",
		"not-a-url",
	}
	for _, raw := range blocked {
		if _, err := ValidateTargetURL(raw, false); err == nil {
			t.Fatalf("%q should be blocked", raw)
		}
	}
}

func TestValidateTargetURLAllowPrivate(t *testing.T) {
	if _, err := ValidateTargetURL("http://10.0.0.5/internal", true); err != nil {
		t.Fatalf("expected private target allowed: %v", err)
	}
}
