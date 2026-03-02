package video

import (
	"net/http"

	"go.uber.org/zap"
)

// redirectTransport redirects all requests to a test server.
type redirectTransport struct {
	targetURL string
	inner     http.RoundTripper
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	newURL := t.targetURL + req.URL.Path + "?" + req.URL.RawQuery
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header
	return t.inner.RoundTrip(newReq)
}

func newRunwayProvider(cfg RunwayConfig, logger ...*zap.Logger) *RunwayProvider {
	if len(logger) > 0 {
		return NewRunwayProvider(cfg, logger[0])
	}
	return NewRunwayProvider(cfg, nil)
}

func newSoraProvider(cfg SoraConfig, logger ...*zap.Logger) *SoraProvider {
	if len(logger) > 0 {
		return NewSoraProvider(cfg, logger[0])
	}
	return NewSoraProvider(cfg, nil)
}

func newGeminiProvider(cfg GeminiConfig, logger ...*zap.Logger) *GeminiProvider {
	if len(logger) > 0 {
		return NewGeminiProvider(cfg, logger[0])
	}
	return NewGeminiProvider(cfg, nil)
}

func newVeoProvider(cfg VeoConfig, logger ...*zap.Logger) *VeoProvider {
	if len(logger) > 0 {
		return NewVeoProvider(cfg, logger[0])
	}
	return NewVeoProvider(cfg, nil)
}

func newLumaProvider(cfg LumaConfig, logger ...*zap.Logger) *LumaProvider {
	if len(logger) > 0 {
		return NewLumaProvider(cfg, logger[0])
	}
	return NewLumaProvider(cfg, nil)
}

func newKlingProvider(cfg KlingConfig, logger ...*zap.Logger) *KlingProvider {
	if len(logger) > 0 {
		return NewKlingProvider(cfg, logger[0])
	}
	return NewKlingProvider(cfg, nil)
}

func newMiniMaxVideoProvider(cfg MiniMaxVideoConfig, logger ...*zap.Logger) *MiniMaxVideoProvider {
	if len(logger) > 0 {
		return NewMiniMaxVideoProvider(cfg, logger[0])
	}
	return NewMiniMaxVideoProvider(cfg, nil)
}
