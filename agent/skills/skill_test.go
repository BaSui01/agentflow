package skills

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkill_Validate_MissingID(t *testing.T) {
	s := &Skill{Name: "test", Instructions: "do stuff"}
	assert.Error(t, s.Validate())
}

func TestSkill_Validate_MissingName(t *testing.T) {
	s := &Skill{ID: "s1", Instructions: "do stuff"}
	assert.Error(t, s.Validate())
}

func TestSkill_Validate_MissingInstructions(t *testing.T) {
	s := &Skill{ID: "s1", Name: "test"}
	assert.Error(t, s.Validate())
}

func TestSkill_Validate_SetsDefaultVersion(t *testing.T) {
	s := &Skill{ID: "s1", Name: "test", Instructions: "do stuff"}
	require.NoError(t, s.Validate())
	assert.Equal(t, "1.0.0", s.Version)
}

func TestSkill_Clone_DeepCopy(t *testing.T) {
	s := &Skill{
		ID:           "s1",
		Name:         "test",
		Tags:         []string{"a", "b"},
		Tools:        []string{"t1"},
		Dependencies: []string{"d1"},
		Resources:    map[string]any{"r1": "val"},
		Examples:     []SkillExample{{Input: "i", Output: "o"}},
	}

	clone := s.Clone()
	assert.Equal(t, s.ID, clone.ID)

	// Mutating clone should not affect original
	clone.Tags = append(clone.Tags, "c")
	assert.Len(t, s.Tags, 2)

	clone.Resources["r2"] = "new"
	_, exists := s.Resources["r2"]
	assert.False(t, exists)
}

func TestSkill_MatchesTask(t *testing.T) {
	s := &Skill{
		Name:        "Code Review",
		Description: "Review Go code quality",
		Tags:        []string{"go", "review"},
		Category:    "coding",
	}

	score := s.MatchesTask("review go code")
	assert.Greater(t, score, 0.0)

	zeroScore := s.MatchesTask("deploy kubernetes")
	assert.Less(t, zeroScore, score)
}

func TestSkill_RenderInstructions(t *testing.T) {
	s := &Skill{Instructions: "Hello {{name}}, review {{file}}"}
	result := s.RenderInstructions(map[string]string{"name": "Alice", "file": "main.go"})
	assert.Equal(t, "Hello Alice, review main.go", result)
}

func TestSkill_RenderInstructions_NilVars(t *testing.T) {
	s := &Skill{Instructions: "Hello {{name}}"}
	result := s.RenderInstructions(nil)
	assert.Equal(t, "Hello {{name}}", result)
}

func TestSkill_GetInstructions_NilSkill(t *testing.T) {
	var s *Skill
	assert.Equal(t, "", s.GetInstructions())
}

func TestSkill_GetResourceAsString(t *testing.T) {
	s := &Skill{Resources: map[string]any{
		"text":  "hello",
		"bytes": []byte("world"),
		"obj":   map[string]string{"k": "v"},
	}}

	str, err := s.GetResourceAsString("text")
	require.NoError(t, err)
	assert.Equal(t, "hello", str)

	str, err = s.GetResourceAsString("bytes")
	require.NoError(t, err)
	assert.Equal(t, "world", str)

	str, err = s.GetResourceAsString("obj")
	require.NoError(t, err)
	assert.Contains(t, str, "k")

	_, err = s.GetResourceAsString("missing")
	assert.Error(t, err)
}

func TestSkill_GetResourceAsJSON(t *testing.T) {
	s := &Skill{Resources: map[string]any{
		"json_str": `{"key":"val"}`,
	}}

	var target map[string]string
	require.NoError(t, s.GetResourceAsJSON("json_str", &target))
	assert.Equal(t, "val", target["key"])

	err := s.GetResourceAsJSON("missing", &target)
	assert.Error(t, err)
}

func TestSkill_ToToolSchema(t *testing.T) {
	s := &Skill{ID: "s1", Description: "A skill"}
	schema := s.ToToolSchema()
	assert.Equal(t, "s1", schema.Name)
	assert.Equal(t, "A skill", schema.Description)
	assert.NotEmpty(t, schema.Parameters)
}

func TestSkillBuilder_Build_Success(t *testing.T) {
	skill, err := NewSkillBuilder("s1", "Test Skill").
		WithDescription("A test skill").
		WithInstructions("Do the thing").
		WithCategory("coding").
		WithTags("go", "test").
		WithTools("tool1").
		WithResource("data.txt", "content").
		WithExample("input", "output", "explanation").
		WithPriority(5).
		WithLazyLoad(true).
		WithDependencies("dep1").
		Build()

	require.NoError(t, err)
	assert.Equal(t, "s1", skill.ID)
	assert.Equal(t, "Test Skill", skill.Name)
	assert.Equal(t, "coding", skill.Category)
	assert.Equal(t, 5, skill.Priority)
	assert.True(t, skill.LazyLoad)
	assert.Len(t, skill.Tags, 2)
	assert.Len(t, skill.Dependencies, 1)
}

func TestSkillBuilder_Build_MissingInstructions(t *testing.T) {
	_, err := NewSkillBuilder("s1", "Test").Build()
	assert.Error(t, err)
}

func TestLoadSkillFromDirectory(t *testing.T) {
	dir := t.TempDir()
	skill := &Skill{
		ID:           "test-skill",
		Name:         "Test Skill",
		Version:      "1.0.0",
		Instructions: "Do the thing",
		Resources:    map[string]any{},
		Tools:        []string{},
		Examples:     []SkillExample{},
	}

	require.NoError(t, SaveSkillToDirectory(skill, dir))

	loaded, err := LoadSkillFromDirectory(dir)
	require.NoError(t, err)
	assert.Equal(t, "test-skill", loaded.ID)
	assert.True(t, loaded.Loaded)
}

func TestLoadSkillFromDirectory_NotFound(t *testing.T) {
	_, err := LoadSkillFromDirectory("/nonexistent/path")
	assert.Error(t, err)
}

func TestSaveSkillToDirectory_WithResources(t *testing.T) {
	dir := t.TempDir()
	skill := &Skill{
		ID:           "res-skill",
		Name:         "Resource Skill",
		Version:      "1.0.0",
		Instructions: "Use resources",
		Resources: map[string]any{
			"data.txt": "text content",
		},
		Tools:    []string{},
		Examples: []SkillExample{},
	}

	require.NoError(t, SaveSkillToDirectory(skill, dir))

	loaded, err := LoadSkillFromDirectory(dir)
	require.NoError(t, err)
	assert.Equal(t, "res-skill", loaded.ID)
}

