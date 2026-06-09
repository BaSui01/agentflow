package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHotReloadableFieldsAreBuiltFromStructTags(t *testing.T) {
	fields := GetHotReloadableFields()

	level := fields["Log.Level"]
	require.Equal(t, "Log.Level", level.Path)
	require.False(t, level.RequiresRestart)
	require.False(t, level.Sensitive)
	require.Contains(t, level.Description, "Log level")

	port := fields["Server.HTTPPort"]
	require.Equal(t, "Server.HTTPPort", port.Path)
	require.True(t, port.RequiresRestart)
	require.False(t, port.Sensitive)

	password := fields["Database.Password"]
	require.Equal(t, "Database.Password", password.Path)
	require.True(t, password.RequiresRestart)
	require.True(t, password.Sensitive)

	apiKey := fields["Multimodal.Image.OpenAIAPIKey"]
	require.Equal(t, "Multimodal.Image.OpenAIAPIKey", apiKey.Path)
	require.True(t, apiKey.RequiresRestart)
	require.True(t, apiKey.Sensitive)

	_, found := fields["Log.OutputPaths"]
	require.False(t, found, "fields without reload tags must stay out of the registry")
}

func TestHotReloadableFieldRegistryDoesNotUseHandWrittenPathKeys(t *testing.T) {
	content, err := readConfigLoaderSourceForTest()
	require.NoError(t, err)
	require.NotContains(t, content, "\"Log.Level\": {")
	require.NotContains(t, content, "\"Server.HTTPPort\": {")
	require.Contains(t, content, `reload:"`)
	require.Contains(t, content, `restart:"`)
}

func readConfigLoaderSourceForTest() (string, error) {
	data, err := os.ReadFile("loader.go")
	if err != nil && strings.Contains(err.Error(), "cannot find") {
		data, err = os.ReadFile("config/loader.go")
	}
	return string(data), err
}
