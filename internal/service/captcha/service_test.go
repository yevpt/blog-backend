package captcha

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/dto"
)

type fakeSlideGenerator struct {
	err error
}

func (g fakeSlideGenerator) Generate() (*slideChallenge, error) {
	if g.err != nil {
		return nil, g.err
	}

	return &slideChallenge{
		MasterImage: "data:image/jpeg;base64,master",
		TileImage:   "data:image/png;base64,tile",
		X:           160,
		Y:           82,
		TileX:       12,
		TileY:       82,
		Width:       64,
		Height:      64,
		ImageWidth:  300,
		ImageHeight: 220,
	}, nil
}

func setupCaptchaService(t *testing.T) (*service, *redis.Client, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := newServiceWithGenerator(rdb, fakeSlideGenerator{})

	return svc, rdb, mr
}

func TestServiceGenerateRegistrationChallenge(t *testing.T) {
	svc, rdb, mr := setupCaptchaService(t)
	defer mr.Close()

	resp, err := svc.GenerateRegistrationChallenge()

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.ChallengeID)
	assert.Equal(t, "data:image/jpeg;base64,master", resp.MasterImage)
	assert.Equal(t, "data:image/png;base64,tile", resp.TileImage)
	assert.Equal(t, 12, resp.TileX)
	assert.Equal(t, 82, resp.TileY)
	assert.Equal(t, 300, resp.ImageWidth)
	assert.Equal(t, 220, resp.ImageHeight)

	exists, err := rdb.Exists(context.Background(), challengeKey(resp.ChallengeID)).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists)
}

func TestServiceVerifyRegistrationChallengeRejectsWrongPosition(t *testing.T) {
	svc, _, mr := setupCaptchaService(t)
	defer mr.Close()

	challenge, err := svc.GenerateRegistrationChallenge()
	require.NoError(t, err)

	resp, err := svc.VerifyRegistrationChallenge(&dto.CaptchaVerifyReq{
		ChallengeID: challenge.ChallengeID,
		X:           20,
		Y:           82,
	}, "127.0.0.1")

	assert.ErrorIs(t, err, ErrInvalidCaptcha)
	assert.Nil(t, resp)
}

func TestServiceVerifyRegistrationChallengeReturnsOneTimeToken(t *testing.T) {
	svc, _, mr := setupCaptchaService(t)
	defer mr.Close()

	challenge, err := svc.GenerateRegistrationChallenge()
	require.NoError(t, err)

	resp, err := svc.VerifyRegistrationChallenge(&dto.CaptchaVerifyReq{
		ChallengeID: challenge.ChallengeID,
		X:           162,
		Y:           84,
	}, "127.0.0.1")

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.CaptchaToken)

	err = svc.ConsumeRegistrationToken(resp.CaptchaToken, "127.0.0.1")
	require.NoError(t, err)

	err = svc.ConsumeRegistrationToken(resp.CaptchaToken, "127.0.0.1")
	assert.ErrorIs(t, err, ErrInvalidCaptchaToken)
}

func TestServiceGenerateRegistrationChallengeReturnsGeneratorError(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := newServiceWithGenerator(rdb, fakeSlideGenerator{err: errors.New("boom")})

	resp, err := svc.GenerateRegistrationChallenge()

	assert.Error(t, err)
	assert.Nil(t, resp)
}
