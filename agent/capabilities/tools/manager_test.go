package skills

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestDefaultSkillManager_LoadsRegisteredSkillWhenAutoLoadDisabled(t *testing.T) {
	config := DefaultSkillManagerConfig()
	config.AutoLoad = false

	mgr := NewSkillManager(config, zap.NewNop())
	skill, err := NewSkillBuilder("code-review", "Code Review").
		WithDescription("Review code quality and safety").
		WithInstructions("Review the code and explain issues").
		WithTags("code", "review").
		Build()
	if err != nil {
		t.Fatalf("build skill: %v", err)
	}

	if err := mgr.RegisterSkill(skill); err != nil {
		t.Fatalf("register skill: %v", err)
	}

	if _, loaded := mgr.GetSkill(skill.ID); loaded {
		t.Fatalf("expected skill not auto loaded")
	}

	loadedSkill, err := mgr.LoadSkill(context.Background(), skill.ID)
	if err != nil {
		t.Fatalf("load skill: %v", err)
	}
	if loadedSkill == nil || loadedSkill.ID != skill.ID {
		t.Fatalf("unexpected loaded skill: %#v", loadedSkill)
	}
	if loadedSkill.Instructions == "" {
		t.Fatalf("expected loaded skill instructions")
	}
}

func TestDefaultSkillManager_SearchSkillsByToken(t *testing.T) {
	mgr := NewSkillManager(DefaultSkillManagerConfig(), zap.NewNop())

	codeReview, _ := NewSkillBuilder("code-review", "Code Review").
		WithDescription("Analyze Go code quality and suggest fixes").
		WithInstructions("Review code.").
		WithCategory("coding").
		WithTags("go", "review").
		Build()
	dataAnalysis, _ := NewSkillBuilder("data-analysis", "Data Analysis").
		WithDescription("Analyze metrics and chart trends").
		WithInstructions("Analyze data.").
		WithCategory("data").
		WithTags("analytics").
		Build()

	if err := mgr.RegisterSkill(codeReview); err != nil {
		t.Fatalf("register code review skill: %v", err)
	}
	if err := mgr.RegisterSkill(dataAnalysis); err != nil {
		t.Fatalf("register data analysis skill: %v", err)
	}

	results := mgr.SearchSkills("please review go code")
	if len(results) == 0 {
		t.Fatalf("expected at least one search result")
	}
	if results[0].ID != "code-review" {
		t.Fatalf("expected code-review ranked first, got %s", results[0].ID)
	}
}

func TestDefaultSkillManager_ScanDirectoryDeduplicatesAndRefreshKeepsInMemory(t *testing.T) {
	tempDir := t.TempDir()
	createSkillFixture(t, tempDir, "lint-check", "Lint Check", "Check lint issues")

	mgr := NewSkillManager(DefaultSkillManagerConfig(), zap.NewNop())

	inMemorySkill, err := NewSkillBuilder("in-memory", "In Memory").
		WithDescription("memory only skill").
		WithInstructions("Run in memory checks").
		Build()
	if err != nil {
		t.Fatalf("build in-memory skill: %v", err)
	}
	if err := mgr.RegisterSkill(inMemorySkill); err != nil {
		t.Fatalf("register in-memory skill: %v", err)
	}

	if err := mgr.ScanDirectory(tempDir); err != nil {
		t.Fatalf("first scan failed: %v", err)
	}
	if err := mgr.ScanDirectory(tempDir); err != nil {
		t.Fatalf("second scan failed: %v", err)
	}

	if got := len(mgr.directories); got != 1 {
		t.Fatalf("expected deduplicated directories, got %d", got)
	}

	if err := mgr.RefreshIndex(); err != nil {
		t.Fatalf("refresh index failed: %v", err)
	}

	if _, err := mgr.LoadSkill(context.Background(), "in-memory"); err != nil {
		t.Fatalf("expected in-memory skill to survive refresh, got %v", err)
	}
	if _, err := mgr.LoadSkill(context.Background(), "lint-check"); err != nil {
		t.Fatalf("expected directory skill to be loadable after refresh, got %v", err)
	}
}

func createSkillFixture(t *testing.T, root, id, name, instructions string) {
	t.Helper()

	skillDir := filepath.Join(root, id)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("create skill directory: %v", err)
	}

	manifest := SkillManifest{Skill: Skill{
		ID:           id,
		Name:         name,
		Version:      "1.0.0",
		Description:  name,
		Instructions: instructions,
		Resources:    map[string]any{},
		Tools:        []string{},
		Examples:     []SkillExample{},
	}}

	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.json"), data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
