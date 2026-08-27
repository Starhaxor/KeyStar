package security

import "testing"

func TestValidatePublicHTTPSURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		url  string
		ok   bool
	}{
		{"public HTTPS", "https://hooks.example.com/events", true},
		{"HTTP", "http://hooks.example.com/events", false},
		{"loopback name", "https://localhost/hook", false},
		{"loopback IPv4", "https://127.0.0.1/hook", false},
		{"private IPv4", "https://10.0.0.4/hook", false},
		{"carrier grade NAT", "https://100.64.0.1/hook", false},
		{"benchmark network", "https://198.18.0.1/hook", false},
		{"documentation IPv4", "https://203.0.113.10/hook", false},
		{"link local metadata", "https://169.254.169.254/latest", false},
		{"IPv6 loopback", "https://[::1]/hook", false},
		{"documentation IPv6", "https://[2001:db8::1]/hook", false},
		{"userinfo", "https://user:pass@example.com/hook", false},
		{"fragment", "https://example.com/hook#secret", false},
		{"non TLS port", "https://example.com:8080/hook", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidatePublicHTTPSURL(test.url)
			if (err == nil) != test.ok {
				t.Fatalf("ValidatePublicHTTPSURL(%q) error = %v, want ok=%v", test.url, err, test.ok)
			}
		})
	}
}
