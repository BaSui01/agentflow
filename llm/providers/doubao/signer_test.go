package doubao

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVolcSigner_Sign(t *testing.T) {
	signer := newVolcSigner("test-ak", "test-sk", "cn-beijing")

	req, err := http.NewRequest(http.MethodPost, "https://ark.cn-beijing.volces.com/api/v3/chat/completions", nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	bodyHash := hashSHA256("{}")
	signer.sign(req, bodyHash)

	auth := req.Header.Get("Authorization")
	assert.Contains(t, auth, "HMAC-SHA256")
	assert.Contains(t, auth, "Credential=test-ak/")
	assert.Contains(t, auth, "cn-beijing/ark/request")
	assert.NotEmpty(t, req.Header.Get("X-Date"))
	assert.NotEmpty(t, req.Header.Get("X-Content-Sha256"))
}

func TestVolcSigner_DefaultRegion(t *testing.T) {
	signer := newVolcSigner("ak", "sk", "")
	assert.Equal(t, "cn-beijing", signer.region)
}

func TestHashSHA256(t *testing.T) {
	// 空字符串的 SHA256
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", hashSHA256(""))
}

func TestHmacSHA256(t *testing.T) {
	result := hmacSHA256([]byte("key"), "data")
	assert.NotEmpty(t, result)
	assert.Len(t, result, 32) // SHA256 produces 32 bytes
}

