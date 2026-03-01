package doubao

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	defaultRegion   = "cn-beijing"
	serviceName     = "ark"
	signingAlgo     = "HMAC-SHA256"
	iso8601Layout   = "20060102T150405Z"
	shortDateLayout = "20060102"
)

// volcSigner 实现火山引擎 HMAC-SHA256 请求签名。
type volcSigner struct {
	accessKey string
	secretKey string
	region    string
}

func newVolcSigner(ak, sk, region string) *volcSigner {
	if region == "" {
		region = defaultRegion
	}
	return &volcSigner{accessKey: ak, secretKey: sk, region: region}
}

// sign 对 HTTP 请求进行签名，添加 Authorization 和相关头。
func (s *volcSigner) sign(req *http.Request, bodyHash string) {
	now := time.Now().UTC()
	dateStamp := now.Format(shortDateLayout)
	amzDate := now.Format(iso8601Layout)

	req.Header.Set("X-Date", amzDate)
	req.Header.Set("X-Content-Sha256", bodyHash)

	// 1. 构建 Canonical Request
	signedHeaders, canonicalHeaders := s.buildCanonicalHeaders(req)
	canonicalRequest := strings.Join([]string{
		req.Method,
		req.URL.Path,
		req.URL.RawQuery,
		canonicalHeaders,
		signedHeaders,
		bodyHash,
	}, "\n")

	// 2. 构建 String to Sign
	credentialScope := fmt.Sprintf("%s/%s/%s/request", dateStamp, s.region, serviceName)
	stringToSign := strings.Join([]string{
		signingAlgo,
		amzDate,
		credentialScope,
		hashSHA256(canonicalRequest),
	}, "\n")

	// 3. 计算签名
	signingKey := s.deriveSigningKey(dateStamp)
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	// 4. 设置 Authorization 头
	authHeader := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		signingAlgo, s.accessKey, credentialScope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)
}

func (s *volcSigner) deriveSigningKey(dateStamp string) []byte {
	kDate := hmacSHA256([]byte(s.secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, s.region)
	kService := hmacSHA256(kRegion, serviceName)
	kSigning := hmacSHA256(kService, "request")
	return kSigning
}

func (s *volcSigner) buildCanonicalHeaders(req *http.Request) (signedHeaders, canonicalHeaders string) {
	// 需要签名的头：host, content-type, x-date, x-content-sha256
	headers := map[string]string{
		"host":             req.Host,
		"content-type":     req.Header.Get("Content-Type"),
		"x-date":           req.Header.Get("X-Date"),
		"x-content-sha256": req.Header.Get("X-Content-Sha256"),
	}

	// 按 key 排序
	keys := make([]string, 0, len(headers))
	for k := range headers {
		if headers[k] != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var canonicalParts []string
	var signedParts []string
	for _, k := range keys {
		canonicalParts = append(canonicalParts, fmt.Sprintf("%s:%s", k, strings.TrimSpace(headers[k])))
		signedParts = append(signedParts, k)
	}

	canonicalHeaders = strings.Join(canonicalParts, "\n") + "\n"
	signedHeaders = strings.Join(signedParts, ";")
	return
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func hashSHA256(data string) string {
	h := sha256.New()
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

