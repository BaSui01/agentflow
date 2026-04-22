package agent

import "strings"

type CodeValidationLanguage string

const (
	CodeLangPython     CodeValidationLanguage = "python"
	CodeLangJavaScript CodeValidationLanguage = "javascript"
	CodeLangTypeScript CodeValidationLanguage = "typescript"
	CodeLangGo         CodeValidationLanguage = "go"
	CodeLangRust       CodeValidationLanguage = "rust"
	CodeLangBash       CodeValidationLanguage = "bash"
)

type CodeValidator struct{}

func NewCodeValidator() *CodeValidator {
	return &CodeValidator{}
}

func (v *CodeValidator) Validate(lang CodeValidationLanguage, code string) []string {
	if strings.TrimSpace(code) == "" {
		return nil
	}
	patterns := map[CodeValidationLanguage][]string{
		CodeLangPython:     {"import os", "os.system", "subprocess.", "eval(", "exec("},
		CodeLangJavaScript: {"require('child_process')", "require(\"child_process\")", "child_process", "eval(", "new Function("},
		CodeLangTypeScript: {"require('child_process')", "require(\"child_process\")", "child_process", "eval(", "new Function("},
		CodeLangGo:         {"os/exec", "exec.Command", "syscall."},
		CodeLangRust:       {"unsafe", "std::process::Command", "libc::"},
		CodeLangBash:       {"rm -rf", "curl ", "wget ", "chmod 777", "sudo "},
	}
	checks := patterns[lang]
	if len(checks) == 0 {
		return nil
	}
	warnings := make([]string, 0, len(checks))
	seen := make(map[string]struct{}, len(checks))
	for _, needle := range checks {
		if strings.Contains(code, needle) {
			if _, ok := seen[needle]; ok {
				continue
			}
			seen[needle] = struct{}{}
			warnings = append(warnings, "potentially dangerous pattern: "+needle)
		}
	}
	return warnings
}
