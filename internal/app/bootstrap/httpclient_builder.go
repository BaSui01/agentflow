package bootstrap

import (
	"time"

	"github.com/BaSui01/agentflow/pkg/httpclient"
)

func BuildHTTPClientFactory() *httpclient.Factory {
	return httpclient.NewFactory(
		httpclient.WithTimeout(30*time.Second),
		httpclient.WithMaxIdleConns(100),
	)
}
