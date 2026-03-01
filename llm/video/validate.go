package video

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateGenerateRequest validates common fields of a GenerateRequest.
// Returns an error if the request is invalid.
func ValidateGenerateRequest(req *GenerateRequest) error {
	if req == nil {
		return fmt.Errorf("generate request must not be nil")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return fmt.Errorf("prompt must not be empty")
	}
	if req.ImageURL != "" {
		if err := ValidateExternalURL(req.ImageURL); err != nil {
			return fmt.Errorf("invalid image_url: %w", err)
		}
	}
	return nil
}

// ValidateExternalURL checks that a URL is a valid external HTTP(S) URL,
// rejecting file://, internal IPs, and loopback addresses (SSRF protection).
func ValidateExternalURL(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("malformed URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL must include a valid host")
	}
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
			ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("URL must not point to a private or loopback address")
		}
	} else {
		lower := strings.ToLower(host)
		if lower == "localhost" || strings.HasSuffix(lower, ".local") {
			return fmt.Errorf("URL must not point to a local host")
		}
	}
	return nil
}
