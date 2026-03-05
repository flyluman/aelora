package valkey

import (
	"testing"
)

func TestSplitToken(t *testing.T) {
	tests := []struct {
		input        string
		wantPlatform string
		wantToken    string
		wantErr      bool
	}{
		{"android|abc123", "android", "abc123", false},
		{"ios|xyz789", "ios", "xyz789", false},
		{"", "", "", true},
		{"invalid", "", "", true},
		{"|empty", "", "", true},
	}

	for _, tt := range tests {
		platform, token, err := splitToken(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("splitToken(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
			continue
		}
		if platform != tt.wantPlatform || token != tt.wantToken {
			t.Errorf("splitToken(%q) = (%q, %q), want (%q, %q)", tt.input, platform, token, tt.wantPlatform, tt.wantToken)
		}
	}
}

func TestKeys(t *testing.T) {
	if k := presenceKey("u1", "d1"); k != "presence:u1:d1" {
		t.Fatalf("unexpected presence key: %s", k)
	}
	if k := presenceDevicesKey("u1"); k != "presence_devices:u1" {
		t.Fatalf("unexpected presence devices key: %s", k)
	}
	if k := pushTokenKey("u1"); k != "push_tokens:u1" {
		t.Fatalf("unexpected push token key: %s", k)
	}
	if k := ackKey("u1", "d1", "r1"); k != "last_ack:u1:d1:r1" {
		t.Fatalf("unexpected ack key: %s", k)
	}
	if k := roomMembersKey("r1"); k != "room:r1:members" {
		t.Fatalf("unexpected room members key: %s", k)
	}
	if k := roomMetaKey("r1"); k != "room:r1:meta" {
		t.Fatalf("unexpected room meta key: %s", k)
	}
	if k := StreamKey("r1"); k != "stream:room:r1" {
		t.Fatalf("unexpected stream key: %s", k)
	}
}
