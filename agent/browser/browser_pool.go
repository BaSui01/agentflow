package browser

import "go.uber.org/zap"

// ChromeDPBrowserFactory implements BrowserFactory for production wiring.
type ChromeDPBrowserFactory struct {
	logger *zap.Logger
}

// NewChromeDPBrowserFactory creates a browser factory.
func NewChromeDPBrowserFactory(logger *zap.Logger) *ChromeDPBrowserFactory {
	return &ChromeDPBrowserFactory{logger: logger}
}

// Create constructs a browser instance from runtime config.
func (f *ChromeDPBrowserFactory) Create(config BrowserConfig) (Browser, error) {
	return NewChromeDPBrowser(config, f.logger)
}
