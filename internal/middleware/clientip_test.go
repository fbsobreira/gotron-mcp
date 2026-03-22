package middleware

import (
	"net/http"
	"testing"
)

func TestParseTrustMode(t *testing.T) {
	tests := []struct {
		input string
		want  TrustMode
	}{
		{"none", TrustNone},
		{"all", TrustAll},
		{"cloudflare", TrustCloudflare},
		{"Cloudflare", TrustCloudflare},
		{"ALL", TrustAll},
		{"", TrustNone},
		{"unknown", TrustNone},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseTrustMode(tt.input)
			if got != tt.want {
				t.Errorf("ParseTrustMode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		mode       TrustMode
		want       string
	}{
		{
			name:       "none mode uses RemoteAddr",
			remoteAddr: "1.2.3.4:12345",
			headers:    map[string]string{"CF-Connecting-IP": "5.6.7.8"},
			mode:       TrustNone,
			want:       "1.2.3.4",
		},
		{
			name:       "all mode trusts CF-Connecting-IP",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"CF-Connecting-IP": "5.6.7.8"},
			mode:       TrustAll,
			want:       "5.6.7.8",
		},
		{
			name:       "all mode trusts X-Real-IP when no CF header",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"X-Real-IP": "9.10.11.12"},
			mode:       TrustAll,
			want:       "9.10.11.12",
		},
		{
			name:       "all mode trusts X-Forwarded-For first IP",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"X-Forwarded-For": "1.1.1.1, 2.2.2.2"},
			mode:       TrustAll,
			want:       "1.1.1.1",
		},
		{
			name:       "all mode falls back to RemoteAddr when no headers",
			remoteAddr: "10.0.0.1:12345",
			headers:    nil,
			mode:       TrustAll,
			want:       "10.0.0.1",
		},
		{
			name:       "cloudflare mode trusts headers from loopback",
			remoteAddr: "127.0.0.1:12345",
			headers:    map[string]string{"CF-Connecting-IP": "8.8.8.8"},
			mode:       TrustCloudflare,
			want:       "8.8.8.8",
		},
		{
			name:       "cloudflare mode ignores headers from non-loopback",
			remoteAddr: "1.2.3.4:12345",
			headers:    map[string]string{"CF-Connecting-IP": "8.8.8.8"},
			mode:       TrustCloudflare,
			want:       "1.2.3.4",
		},
		{
			name:       "cloudflare mode trusts from IPv6 loopback",
			remoteAddr: "[::1]:12345",
			headers:    map[string]string{"CF-Connecting-IP": "8.8.4.4"},
			mode:       TrustCloudflare,
			want:       "8.8.4.4",
		},
		{
			name:       "CF-Connecting-IP takes priority over X-Real-IP",
			remoteAddr: "127.0.0.1:12345",
			headers: map[string]string{
				"CF-Connecting-IP": "1.1.1.1",
				"X-Real-IP":        "2.2.2.2",
			},
			mode: TrustCloudflare,
			want: "1.1.1.1",
		},
		{
			name:       "X-Real-IP takes priority over X-Forwarded-For",
			remoteAddr: "10.0.0.1:12345",
			headers: map[string]string{
				"X-Real-IP":       "3.3.3.3",
				"X-Forwarded-For": "4.4.4.4",
			},
			mode: TrustAll,
			want: "3.3.3.3",
		},
		{
			name:       "RemoteAddr without port",
			remoteAddr: "1.2.3.4",
			headers:    nil,
			mode:       TrustNone,
			want:       "1.2.3.4",
		},
		{
			name:       "invalid CF-Connecting-IP falls back to RemoteAddr",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"CF-Connecting-IP": "not-an-ip"},
			mode:       TrustAll,
			want:       "10.0.0.1",
		},
		{
			name:       "invalid X-Real-IP falls back to RemoteAddr",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"X-Real-IP": "garbage"},
			mode:       TrustAll,
			want:       "10.0.0.1",
		},
		{
			name:       "invalid X-Forwarded-For falls back to RemoteAddr",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"X-Forwarded-For": "bad, 2.2.2.2"},
			mode:       TrustAll,
			want:       "10.0.0.1",
		},
		{
			name:       "IPv6 client address via header",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"CF-Connecting-IP": "2001:db8::1"},
			mode:       TrustAll,
			want:       "2001:db8::1",
		},
		{
			name:       "IP with port in header falls back to RemoteAddr",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"X-Real-IP": "1.2.3.4:8080"},
			mode:       TrustAll,
			want:       "10.0.0.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{
				RemoteAddr: tt.remoteAddr,
				Header:     http.Header{},
			}
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			got := ClientIP(r, tt.mode)
			if got != tt.want {
				t.Errorf("ClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
