package main

import "testing"

func TestFrontendBackendURL(t *testing.T) {
	tests := []struct {
		name     string
		httpAddr string
		want     string
		wantErr  bool
	}{
		{
			name:     "port only uses localhost",
			httpAddr: ":8080",
			want:     "http://127.0.0.1:8080",
		},
		{
			name:     "wildcard ipv4 uses localhost",
			httpAddr: "0.0.0.0:9090",
			want:     "http://127.0.0.1:9090",
		},
		{
			name:     "wildcard ipv6 uses localhost",
			httpAddr: "[::]:7070",
			want:     "http://127.0.0.1:7070",
		},
		{
			name:     "explicit host is preserved",
			httpAddr: "127.0.0.1:6060",
			want:     "http://127.0.0.1:6060",
		},
		{
			name:     "missing port fails",
			httpAddr: "127.0.0.1",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := frontendBackendURL(tt.httpAddr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.httpAddr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected url: got %q want %q", got, tt.want)
			}
		})
	}
}
