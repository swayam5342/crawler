package urlutil

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			name: "strips fragment",
			in:   "https://Example.com/path#section",
			want: "https://example.com/path",
		},
		{
			name: "sorts query params",
			in:   "https://example.com/path?b=2&a=1",
			want: "https://example.com/path?a=1&b=2",
		},
		{
			name: "sorts repeated query keys",
			in:   "https://example.com/?tag=b&tag=a",
			want: "https://example.com/?tag=a&tag=b",
		},
		{
			name: "strips default https port",
			in:   "https://example.com:443/path",
			want: "https://example.com/path",
		},
		{
			name: "strips default http port",
			in:   "http://example.com:80/path",
			want: "http://example.com/path",
		},
		{
			name: "keeps non-default port",
			in:   "https://example.com:8443/path",
			want: "https://example.com:8443/path",
		},
		{
			name: "lower-cases scheme and host",
			in:   "HTTPS://EXAMPLE.COM/Path",
			want: "https://example.com/Path",
		},
		{
			name: "trims trailing slash on non-root path",
			in:   "https://example.com/path/",
			want: "https://example.com/path",
		},
		{
			name: "keeps root path slash",
			in:   "https://example.com",
			want: "https://example.com/",
		},
		{
			name: "keeps root path slash when already present",
			in:   "https://example.com/",
			want: "https://example.com/",
		},
		{
			name:    "relative url is an error",
			in:      "/just/a/path",
			wantErr: true,
		},
		{
			name:    "empty string is an error",
			in:      "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Normalize(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDomain(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://Example.com/path", "example.com"},
		{"http://sub.example.com:8080/", "sub.example.com"},
	}
	for _, tt := range tests {
		got, err := Domain(tt.in)
		if err != nil {
			t.Fatalf("Domain(%q) unexpected error: %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("Domain(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
