package video

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

var aspectRatioPattern = regexp.MustCompile(`^\d+:\d+$`)

const maxVideoPromptLength = 4000

var allowedResolutions = map[string]struct{}{
	"480p":  {},
	"720p":  {},
	"1080p": {},
}

// ValidateGenerateRequest validates common fields of a GenerateRequest.
// Returns an error if the request is invalid.
func ValidateGenerateRequest(req *GenerateRequest) error {
	if req == nil {
		return fmt.Errorf("generate request must not be nil")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return fmt.Errorf("prompt must not be empty")
	}
	if utf8.RuneCountInString(req.Prompt) > maxVideoPromptLength {
		return fmt.Errorf("prompt exceeds max length (%d characters)", maxVideoPromptLength)
	}
	if req.Duration < 0 {
		return fmt.Errorf("duration must be non-negative")
	}
	if req.AspectRatio != "" {
		aspectRatio := strings.TrimSpace(req.AspectRatio)
		if !aspectRatioPattern.MatchString(aspectRatio) {
			return fmt.Errorf("aspect_ratio must follow N:M format")
		}
	}
	if req.Resolution != "" {
		resolution := strings.ToLower(strings.TrimSpace(req.Resolution))
		if _, ok := allowedResolutions[resolution]; !ok {
			return fmt.Errorf("resolution must be one of 480p, 720p, 1080p")
		}
	}
	if req.ImageURL != "" {
		imageURL := strings.TrimSpace(req.ImageURL)
		if strings.HasPrefix(strings.ToLower(imageURL), "data:image/") {
			return nil
		}
		if err := ValidateExternalURL(imageURL); err != nil {
			return fmt.Errorf("invalid image_url: %w", err)
		}
	}
	return nil
}

func validateAllowedModel(provider string, model string, allowedModels map[string]struct{}) error {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return fmt.Errorf("%s model must not be empty", provider)
	}
	if len(allowedModels) == 0 {
		return nil
	}
	if _, ok := allowedModels[trimmed]; !ok {
		return fmt.Errorf("%s model %q is not allowed", provider, trimmed)
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
