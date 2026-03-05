package push

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestNotifyMessageSkipsWhenEndpointMissing(t *testing.T) {
	n := NewHTTPNotifier("", "", "", time.Second)
	err := n.NotifyMessage(context.Background(), ports.PushToken{
		UserID: "u1", DeviceID: "d1", Platform: ports.PushPlatformAndroid, Token: "tok",
	}, domain.Message{ID: "m1", RoomID: "r1", SenderID: "u2", Content: "hello", ClientMsgID: "c1"})
	if err != nil {
		t.Fatalf("expected nil error when endpoint is empty, got %v", err)
	}
}

func TestNotifyMessagePostsPayload(t *testing.T) {
	n := NewHTTPNotifier("http://push.local/send", "", "abc", time.Second)
	n.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	err := n.NotifyMessage(context.Background(), ports.PushToken{
		UserID: "u1", DeviceID: "d1", Platform: ports.PushPlatformAndroid, Token: "tok",
	}, domain.Message{ID: "m1", RoomID: "r1", SenderID: "u2", Content: "hello", ClientMsgID: "c1"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
