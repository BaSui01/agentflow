package multimodal

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Content factory tests ---

func TestNewTextContent(t *testing.T) {
	c := NewTextContent("hello")
	assert.Equal(t, ContentTypeText, c.Type)
	assert.Equal(t, "hello", c.Text)
}

func TestNewImageURLContent(t *testing.T) {
	c := NewImageURLContent("https://example.com/img.png")
	assert.Equal(t, ContentTypeImage, c.Type)
	assert.Equal(t, "https://example.com/img.png", c.ImageURL)
}

func TestNewImageBase64Content(t *testing.T) {
	c := NewImageBase64Content("abc123", ImageFormatPNG)
	assert.Equal(t, ContentTypeImage, c.Type)
	assert.Equal(t, "abc123", c.Data)
	assert.Equal(t, "image/png", c.MediaType)
}

func TestNewAudioURLContent(t *testing.T) {
	c := NewAudioURLContent("https://example.com/audio.mp3")
	assert.Equal(t, ContentTypeAudio, c.Type)
	assert.Equal(t, "https://example.com/audio.mp3", c.AudioURL)
}

func TestNewAudioBase64Content(t *testing.T) {
	c := NewAudioBase64Content("audiodata", AudioFormatMP3)
	assert.Equal(t, ContentTypeAudio, c.Type)
	assert.Equal(t, "audiodata", c.Data)
	assert.Equal(t, "audio/mp3", c.MediaType)
}

// --- Config defaults tests ---

func TestDefaultVisionConfig(t *testing.T) {
	cfg := DefaultVisionConfig()
	assert.Equal(t, ResolutionAuto, cfg.Resolution)
	assert.Equal(t, int64(20*1024*1024), cfg.MaxImageSize)
	assert.Equal(t, 4096, cfg.MaxDimension)
	assert.Len(t, cfg.AllowedFormats, 4)
}

func TestDefaultAudioConfig(t *testing.T) {
	cfg := DefaultAudioConfig()
	assert.Equal(t, float64(300), cfg.MaxDuration)
	assert.Equal(t, int64(25*1024*1024), cfg.MaxFileSize)
	assert.Equal(t, 16000, cfg.SampleRate)
	assert.Len(t, cfg.AllowedFormats, 5)
}

// --- LoadImageFromFile tests ---

func TestLoadImageFromFile(t *testing.T) {
	tests := []struct {
		name     string
		ext      string
		magic    []byte
		wantType string
	}{
		{"png file", ".png", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, "image/png"},
		{"jpeg file", ".jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, "image/jpeg"},
		{"gif file", ".gif", []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61}, "image/gif"},
		{"webp file", ".webp", []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50}, "image/webp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "test"+tt.ext)
			require.NoError(t, os.WriteFile(path, tt.magic, 0644))

			content, err := LoadImageFromFile(path)
			require.NoError(t, err)
			assert.Equal(t, ContentTypeImage, content.Type)
			assert.Equal(t, tt.wantType, content.MediaType)
			assert.Equal(t, "test"+tt.ext, content.FileName)
			assert.Equal(t, int64(len(tt.magic)), content.FileSize)
			assert.NotEmpty(t, content.Data)
		})
	}
}

func TestLoadImageFromFile_UnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bmp")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0644))

	_, err := LoadImageFromFile(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported image format")
}

func TestLoadImageFromFile_NotFound(t *testing.T) {
	_, err := LoadImageFromFile("/nonexistent/path/image.png")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read image file")
}

// --- LoadImageFromURL tests ---

func TestLoadImageFromURL(t *testing.T) {
	pngMagic := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngMagic)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AGENTFLOW_ALLOW_PRIVATE_URLS", "1")

	content, err := LoadImageFromURL(srv.URL + "/image.png")
	require.NoError(t, err)
	assert.Equal(t, ContentTypeImage, content.Type)
	assert.Equal(t, "image/png", content.MediaType)
	assert.Equal(t, base64.StdEncoding.EncodeToString(pngMagic), content.Data)
}

func TestLoadImageFromURL_DetectFromMagicBytes(t *testing.T) {
	jpegMagic := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(jpegMagic)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AGENTFLOW_ALLOW_PRIVATE_URLS", "1")

	content, err := LoadImageFromURL(srv.URL + "/image")
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", content.MediaType)
}

func TestLoadImageFromURL_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AGENTFLOW_ALLOW_PRIVATE_URLS", "1")

	_, err := LoadImageFromURL(srv.URL + "/missing.png")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 404")
}

func TestLoadImageFromURL_RejectsInternalNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	_, err := LoadImageFromURL(srv.URL + "/image.png")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "URL validation failed")
	assert.Contains(t, err.Error(), "internal network")
}

// --- LoadAudioFromFile tests ---

func TestLoadAudioFromFile(t *testing.T) {
	tests := []struct {
		name     string
		ext      string
		wantType string
	}{
		{"mp3", ".mp3", "audio/mp3"},
		{"wav", ".wav", "audio/wav"},
		{"ogg", ".ogg", "audio/ogg"},
		{"flac", ".flac", "audio/flac"},
		{"m4a", ".m4a", "audio/m4a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "audio"+tt.ext)
			require.NoError(t, os.WriteFile(path, []byte("fake audio data"), 0644))

			content, err := LoadAudioFromFile(path)
			require.NoError(t, err)
			assert.Equal(t, ContentTypeAudio, content.Type)
			assert.Equal(t, tt.wantType, content.MediaType)
			assert.Equal(t, "audio"+tt.ext, content.FileName)
		})
	}
}

func TestLoadAudioFromFile_UnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audio.xyz")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0644))

	_, err := LoadAudioFromFile(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported audio format")
}

// --- detectImageFormat tests ---

func TestDetectImageFormat(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected ImageFormat
	}{
		{"PNG magic", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, ImageFormatPNG},
		{"JPEG magic", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46}, ImageFormatJPEG},
		{"GIF magic", []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x00, 0x00}, ImageFormatGIF},
		{"WebP magic", []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50}, ImageFormatWebP},
		{"too short defaults to JPEG", []byte{0x00, 0x01}, ImageFormatJPEG},
		{"unknown defaults to JPEG", []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}, ImageFormatJPEG},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, detectImageFormat(tt.data))
		})
	}
}

// --- Helper function tests ---

func TestExtractFormat(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"audio/mp3", "mp3"},
		{"audio/wav", "wav"},
		{"image/png", "png"},
		{"text/plain", "text/plain"},
		{"short", "short"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractFormat(tt.input))
		})
	}
}

func TestJoinStrings(t *testing.T) {
	assert.Equal(t, "", joinStrings(nil, ","))
	assert.Equal(t, "a", joinStrings([]string{"a"}, ","))
	assert.Equal(t, "a,b,c", joinStrings([]string{"a", "b", "c"}, ","))
	assert.Equal(t, "a\nb", joinStrings([]string{"a", "b"}, "\n"))
}

// --- Processor tests ---

func TestNewProcessor(t *testing.T) {
	p := NewProcessor(DefaultVisionConfig(), DefaultAudioConfig())
	require.NotNil(t, p)
}

func TestDefaultProcessor(t *testing.T) {
	p := DefaultProcessor()
	require.NotNil(t, p)
}

func TestProcessor_ConvertToOpenAI(t *testing.T) {
	p := DefaultProcessor()
	messages := []MultimodalMessage{
		{
			Role: "user",
			Contents: []Content{
				NewTextContent("describe this"),
				NewImageURLContent("https://example.com/img.png"),
				NewImageBase64Content("b64data", ImageFormatPNG),
				NewAudioBase64Content("audiodata", AudioFormatMP3),
			},
		},
	}

	result, err := p.ConvertToProviderFormat("openai", messages)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "user", string(result[0].Role))

	var parts []map[string]any
	require.NoError(t, json.Unmarshal([]byte(result[0].Content), &parts))
	assert.Len(t, parts, 4)
	assert.Equal(t, "text", parts[0]["type"])
	assert.Equal(t, "image_url", parts[1]["type"])
	assert.Equal(t, "image_url", parts[2]["type"])
	assert.Equal(t, "input_audio", parts[3]["type"])
}

func TestProcessor_ConvertToAnthropic(t *testing.T) {
	p := DefaultProcessor()
	messages := []MultimodalMessage{
		{
			Role: "user",
			Contents: []Content{
				NewTextContent("describe"),
				NewImageBase64Content("b64data", ImageFormatPNG),
				NewImageURLContent("https://example.com/img.png"),
			},
		},
	}

	result, err := p.ConvertToProviderFormat("anthropic", messages)
	require.NoError(t, err)
	require.Len(t, result, 1)

	var parts []map[string]any
	require.NoError(t, json.Unmarshal([]byte(result[0].Content), &parts))
	assert.Len(t, parts, 3)
	assert.Equal(t, "text", parts[0]["type"])
	assert.Equal(t, "image", parts[1]["type"])

	// Check base64 source
	source := parts[1]["source"].(map[string]any)
	assert.Equal(t, "base64", source["type"])

	// Check URL source
	source2 := parts[2]["source"].(map[string]any)
	assert.Equal(t, "url", source2["type"])
}

func TestProcessor_ConvertToGemini(t *testing.T) {
	p := DefaultProcessor()
	messages := []MultimodalMessage{
		{
			Role: "user",
			Contents: []Content{
				NewTextContent("describe"),
				NewImageBase64Content("imgdata", ImageFormatPNG),
				NewImageURLContent("https://example.com/img.png"),
				NewAudioBase64Content("audiodata", AudioFormatMP3),
				{Type: ContentTypeVideo, VideoURL: "https://example.com/video.mp4"},
			},
		},
	}

	result, err := p.ConvertToProviderFormat("gemini", messages)
	require.NoError(t, err)
	require.Len(t, result, 1)

	var parts []map[string]any
	require.NoError(t, json.Unmarshal([]byte(result[0].Content), &parts))
	assert.Len(t, parts, 5)
}

func TestProcessor_ConvertToGeneric(t *testing.T) {
	p := DefaultProcessor()
	messages := []MultimodalMessage{
		{
			Role: "user",
			Contents: []Content{
				NewTextContent("hello"),
				{Type: ContentTypeImage, FileName: "photo.png"},
			},
		},
	}

	result, err := p.ConvertToProviderFormat("unknown-provider", messages)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Contains(t, result[0].Content, "hello")
	assert.Contains(t, result[0].Content, "[image content: photo.png]")
}
