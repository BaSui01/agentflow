package bootstrap

import (
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/types"
)

// BuildModelCatalog loads the optional runtime model catalog snapshot.
// Empty paths deliberately fall back to the built-in snapshot so startup keeps
// a deterministic catalog even before an operator wires generated model data.
func BuildModelCatalog(path string) (*types.ModelCatalog, error) {
	if strings.TrimSpace(path) == "" {
		return types.DefaultModelCatalog(), nil
	}
	catalog, err := types.LoadModelCatalogFile(path)
	if err != nil {
		return nil, fmt.Errorf("build model catalog: %w", err)
	}
	return catalog, nil
}
