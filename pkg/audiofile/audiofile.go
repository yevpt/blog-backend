package audiofile

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

var (
	ErrInvalidAudio  = errors.New("音频文件无效")
	ErrAudioTooLarge = errors.New("音频文件过大")
)

type Result struct {
	Data        []byte
	ContentType string
	Ext         string
	SHA256      string
	Size        uint64
}

func Validate(name string, data []byte, maxBytes int) (Result, error) {
	if maxBytes > 0 && len(data) > maxBytes {
		return Result{}, ErrAudioTooLarge
	}
	contentType, ext := detectAudio(name, data)
	if contentType == "" {
		return Result{}, ErrInvalidAudio
	}
	sum := sha256.Sum256(data)
	return Result{
		Data:        data,
		ContentType: contentType,
		Ext:         ext,
		SHA256:      hex.EncodeToString(sum[:]),
		Size:        uint64(len(data)),
	}, nil
}

func detectAudio(name string, data []byte) (string, string) {
	lower := strings.ToLower(name)
	if len(data) >= 3 && string(data[:3]) == "ID3" {
		return "audio/mpeg", ".mp3"
	}
	if len(data) >= 2 && data[0] == 0xff && data[1]&0xe0 == 0xe0 {
		return "audio/mpeg", ".mp3"
	}
	if len(data) >= 12 && string(data[4:8]) == "ftyp" && (strings.HasSuffix(lower, ".m4a") || strings.HasSuffix(lower, ".mp4")) {
		return "audio/mp4", ".m4a"
	}
	if len(data) >= 4 && string(data[:4]) == "fLaC" {
		return "audio/flac", ".flac"
	}
	return "", ""
}
