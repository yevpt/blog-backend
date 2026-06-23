package notification_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
)

// Stream 在 hub 推送后应写出 event: notification 行。
func TestStream_ReceivesHubNotification(t *testing.T) {
	hub := notificationservice.NewSSEHub()
	srv := httptest.NewServer(newStreamRouter(hub))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/notifications/stream", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	reader := bufio.NewReader(resp.Body)
	waitForLine(t, reader, ": connected")

	hub.NotifyInbox(context.Background(), 7, "reply_created")

	body := readUntilContains(t, reader, "data: reply_created", 2*time.Second)
	assert.Contains(t, body, "event: notification")
	assert.Contains(t, body, "data: reply_created")

	cancel()
}

func waitForLine(t *testing.T, reader *bufio.Reader, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			require.NoError(t, err)
		}
		if strings.TrimSpace(line) == want {
			return
		}
	}
	t.Fatalf("未收到期望行 %q", want)
}

func readUntilContains(t *testing.T, reader *bufio.Reader, substr string, timeout time.Duration) string {
	t.Helper()
	var b strings.Builder
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			require.NoError(t, err)
		}
		b.WriteString(line)
		if strings.Contains(b.String(), substr) {
			return b.String()
		}
	}
	t.Fatalf("超时未收到包含 %q 的输出，已读:\n%s", substr, b.String())
	return ""
}
