package multimodal

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/pkg/tlsutil"
)

var blockedReferenceIPPrefixes = buildBlockedReferenceIPPrefixes()

func buildBlockedReferenceIPPrefixes() []netip.Prefix {
	raw := []string{
		"0.0.0.0/8",       // "this network"
		"100.64.0.0/10",   // carrier-grade NAT
		"192.0.0.0/24",    // IETF protocol assignments
		"192.0.2.0/24",    // TEST-NET-1
		"198.18.0.0/15",   // benchmark testing
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		"224.0.0.0/4",     // multicast
		"240.0.0.0/4",     // reserved
		"2001:db8::/32",   // documentation
	}
	prefixes := make([]netip.Prefix, 0, len(raw))
	for _, cidr := range raw {
		p, err := netip.ParsePrefix(cidr)
		if err != nil {
			continue
		}
		prefixes = append(prefixes, p)
	}
	return prefixes
}

func ValidatePublicReferenceImageURL(ctx context.Context, rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	u, err := url.Parse(trimmed)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("reference_image_url must be a valid http/https URL")
	}

	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("reference_image_url must include a valid host")
	}
	if err := validatePublicHost(ctx, host); err != nil {
		return "", err
	}
	return u.String(), nil
}

func DownloadReferenceImage(ctx context.Context, rawURL string, maxSize int64) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create image download request: %w", err)
	}
	client := newReferenceDownloadHTTPClient(20 * time.Second)
	client.CheckRedirect = func(redirReq *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects while downloading reference image")
		}
		_, validateErr := ValidatePublicReferenceImageURL(ctx, redirReq.URL.String())
		return validateErr
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download reference image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("failed to download reference image: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("failed to read reference image: %w", err)
	}
	if int64(len(data)) > maxSize {
		return nil, "", fmt.Errorf("reference image is too large (max %d bytes)", maxSize)
	}

	mimeType := resp.Header.Get("Content-Type")
	if mediaType, _, parseErr := mime.ParseMediaType(mimeType); parseErr == nil {
		mimeType = mediaType
	}
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return nil, "", fmt.Errorf("reference URL does not point to an image")
	}
	return data, mimeType, nil
}

func validatePublicHost(ctx context.Context, host string) error {
	_, err := resolvePublicHostIPs(ctx, host)
	return err
}

func resolvePublicHostIPs(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if isDisallowedReferenceIP(ip) {
			return nil, fmt.Errorf("reference_image_url must resolve to a public internet address")
		}
		return []net.IP{ip}, nil
	}

	resolveCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(resolveCtx, host)
	if err != nil || len(addrs) == 0 {
		return nil, fmt.Errorf("failed to resolve reference_image_url host")
	}

	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		if isDisallowedReferenceIP(addr.IP) {
			return nil, fmt.Errorf("reference_image_url must resolve to a public internet address")
		}
		ips = append(ips, addr.IP)
	}
	return ips, nil
}

func isDisallowedReferenceIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	if !addr.IsValid() ||
		addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return true
	}
	for _, p := range blockedReferenceIPPrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func newReferenceDownloadHTTPClient(timeout time.Duration) *http.Client {
	base := tlsutil.SecureHTTPClient(timeout)
	transport, ok := base.Transport.(*http.Transport)
	if !ok || transport == nil {
		return base
	}

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	cloned := transport.Clone()
	cloned.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid reference image host: %w", err)
		}
		host = strings.TrimSpace(strings.Trim(host, "[]"))
		ips, err := resolvePublicHostIPs(ctx, host)
		if err != nil {
			return nil, err
		}

		var lastErr error
		for _, ip := range ips {
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr != nil {
			return nil, fmt.Errorf("failed to connect to reference image host: %w", lastErr)
		}
		return nil, fmt.Errorf("failed to connect to reference image host")
	}

	clientCopy := *base
	clientCopy.Transport = cloned
	return &clientCopy
}

