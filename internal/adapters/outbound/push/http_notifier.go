package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
)

type HTTPNotifier struct {
	client          *http.Client
	androidEndpoint string
	iosEndpoint     string
	authToken       string
}

func NewHTTPNotifier(androidEndpoint, iosEndpoint, authToken string, timeout time.Duration) *HTTPNotifier {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &HTTPNotifier{
		client:          &http.Client{Timeout: timeout},
		androidEndpoint: androidEndpoint,
		iosEndpoint:     iosEndpoint,
		authToken:       authToken,
	}
}

func (n *HTTPNotifier) NotifyMessage(ctx context.Context, token ports.PushToken, message domain.Message) error {
	endpoint := ""
	switch token.Platform {
	case ports.PushPlatformAndroid:
		endpoint = n.androidEndpoint
	case ports.PushPlatformIOS:
		endpoint = n.iosEndpoint
	default:
		return errors.New("unsupported push platform")
	}
	if endpoint == "" {
		return nil
	}

	payload := map[string]any{
		"user_id":   token.UserID,
		"device_id": token.DeviceID,
		"token":     token.Token,
		"platform":  token.Platform,
		"message": map[string]string{
			"id":            message.ID,
			"room_id":       message.RoomID,
			"sender_id":     message.SenderID,
			"content":       message.Content,
			"reply_to_id":   message.ReplyToID,
			"client_msg_id": message.ClientMsgID,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if n.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+n.authToken)
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("push provider returned non-success status")
	}
	return nil
}
