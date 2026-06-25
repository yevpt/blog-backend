package providers_test

import (
	"context"
	"net"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	gooauth2 "golang.org/x/oauth2"

	"github.com/vpt/blog-backend/internal/oauth/providers"
)

func TestIsDirectExchangeRetryable(t *testing.T) {
	t.Parallel()

	assert.False(t, providers.IsDirectExchangeRetryable(nil))
	assert.False(t, providers.IsDirectExchangeRetryable(&gooauth2.RetrieveError{ErrorCode: "bad_verification_code"}))
	assert.True(t, providers.IsDirectExchangeRetryable(context.DeadlineExceeded))
	assert.True(t, providers.IsDirectExchangeRetryable(syscall.ECONNREFUSED))

	var netErr net.Error = timeoutError{}
	assert.True(t, providers.IsDirectExchangeRetryable(netErr))
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return false }

func TestIsDirectExchangeRetryableRetrieveError(t *testing.T) {
	t.Parallel()

	err := &gooauth2.RetrieveError{ErrorCode: "bad_verification_code"}
	assert.False(t, providers.IsDirectExchangeRetryable(err))
}
