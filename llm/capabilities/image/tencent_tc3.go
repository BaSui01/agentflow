package image

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	tc3Algorithm     = "TC3-HMAC-SHA256"
	tc3ServiceAiart  = "aiart"
	tc3DefaultRegion = "ap-guangzhou"
	tc3AiartVersion  = "2022-12-29"
)

// tc3SignRequest 对腾讯云 API 3.0 请求做 TC3-HMAC-SHA256 签名，并设置 Authorization 及 X-TC-* 头。
// action 如 SubmitTextToImageJob，version 如 2022-12-29；host 为请求 Host，service 为产品名，region 为地域。
func tc3SignRequest(req *http.Request, payload []byte, secretId, secretKey, host, service, region, action, version string) {
	if service == "" {
		service = tc3ServiceAiart
	}
	if region == "" {
		region = tc3DefaultRegion
	}
	now := time.Now().UTC()
	ts := now.Unix()
	dateStamp := now.Format("2006-01-02")
	contentType := "application/json; charset=utf-8"
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", version)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-TC-Region", region)

	payloadHash := sha256Hex(payload)
	signedHeaders := "content-type;host"
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\n", contentType, host)
	canonicalRequest := strings.Join([]string{
		req.Method,
		"/",
		"",
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	hashedCanonicalRequest := sha256Hex([]byte(canonicalRequest))
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", dateStamp, service)
	stringToSign := strings.Join([]string{
		tc3Algorithm,
		strconv.FormatInt(ts, 10),
		credentialScope,
		hashedCanonicalRequest,
	}, "\n")

	// Derive signing key: SecretDate = HMAC("TC3"+SecretKey, Date); SecretService = HMAC(SecretDate, Service); SecretSigning = HMAC(SecretService, "tc3_request")
	keyTC3 := []byte("TC3" + secretKey)
	secretDate := hmacSHA256(keyTC3, dateStamp)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))

	auth := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		tc3Algorithm, secretId, credentialScope, signedHeaders, signature)
	req.Header.Set("Authorization", auth)
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return strings.ToLower(hex.EncodeToString(h[:]))
}
