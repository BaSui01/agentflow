package lsp

// LSP (Language Server Protocol) 核心协议定义
// 基于 Microsoft LSP 规范

// LSPVersion LSP 协议版本
const LSPVersion = "3.17"

// Position 文档位置
type Position struct {
	Line      int `json:"line"`      // 行号（从 0 开始）
	Character int `json:"character"` // 列号（从 0 开始）
}

// Range 文档范围
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location 位置（文件 + 范围）
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// Diagnostic 诊断信息（错误/警告）
type Diagnostic struct {
	Range    Range              `json:"range"`
	Severity DiagnosticSeverity `json:"severity"`
	Code     string             `json:"code,omitempty"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
	Tags     []DiagnosticTag    `json:"tags,omitempty"`
}

// DiagnosticSeverity 诊断严重性
type DiagnosticSeverity int

const (
	SeverityError       DiagnosticSeverity = 1
	SeverityWarning     DiagnosticSeverity = 2
	SeverityInformation DiagnosticSeverity = 3
	SeverityHint        DiagnosticSeverity = 4
)

// DiagnosticTag 诊断标签
type DiagnosticTag int

const (
	TagUnnecessary DiagnosticTag = 1
	TagDeprecated  DiagnosticTag = 2
)

// CompletionItem 代码补全项
type CompletionItem struct {
	Label            string             `json:"label"`
	Kind             CompletionItemKind `json:"kind"`
	Detail           string             `json:"detail,omitempty"`
	Documentation    string             `json:"documentation,omitempty"`
	Deprecated       bool               `json:"deprecated,omitempty"`
	Preselect        bool               `json:"preselect,omitempty"`
	SortText         string             `json:"sortText,omitempty"`
	FilterText       string             `json:"filterText,omitempty"`
	InsertText       string             `json:"insertText,omitempty"`
	TextEdit         *TextEdit          `json:"textEdit,omitempty"`
	AdditionalEdits  []TextEdit         `json:"additionalTextEdits,omitempty"`
	CommitCharacters []string           `json:"commitCharacters,omitempty"`
	Data             interface{}        `json:"data,omitempty"`
}

// CompletionItemKind 补全项类型
type CompletionItemKind int

const (
	CompletionText        CompletionItemKind = 1
	CompletionMethod      CompletionItemKind = 2
	CompletionFunction    CompletionItemKind = 3
	CompletionConstructor CompletionItemKind = 4
	CompletionField       CompletionItemKind = 5
	CompletionVariable    CompletionItemKind = 6
	CompletionClass       CompletionItemKind = 7
	CompletionInterface   CompletionItemKind = 8
	CompletionModule      CompletionItemKind = 9
	CompletionProperty    CompletionItemKind = 10
	CompletionKeyword     CompletionItemKind = 14
	CompletionSnippet     CompletionItemKind = 15
)

// TextEdit 文本编辑
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// Hover 悬停信息
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// MarkupContent 标记内容
type MarkupContent struct {
	Kind  MarkupKind `json:"kind"`
	Value string     `json:"value"`
}

// MarkupKind 标记类型
type MarkupKind string

const (
	MarkupPlainText MarkupKind = "plaintext"
	MarkupMarkdown  MarkupKind = "markdown"
)

// SymbolInformation 符号信息
type SymbolInformation struct {
	Name          string     `json:"name"`
	Kind          SymbolKind `json:"kind"`
	Deprecated    bool       `json:"deprecated,omitempty"`
	Location      Location   `json:"location"`
	ContainerName string     `json:"containerName,omitempty"`
}

// SymbolKind 符号类型
type SymbolKind int

const (
	SymbolFile        SymbolKind = 1
	SymbolModule      SymbolKind = 2
	SymbolNamespace   SymbolKind = 3
	SymbolPackage     SymbolKind = 4
	SymbolClass       SymbolKind = 5
	SymbolMethod      SymbolKind = 6
	SymbolProperty    SymbolKind = 7
	SymbolField       SymbolKind = 8
	SymbolConstructor SymbolKind = 9
	SymbolEnum        SymbolKind = 10
	SymbolInterface   SymbolKind = 11
	SymbolFunction    SymbolKind = 12
	SymbolVariable    SymbolKind = 13
	SymbolConstant    SymbolKind = 14
	SymbolString      SymbolKind = 15
	SymbolNumber      SymbolKind = 16
	SymbolBoolean     SymbolKind = 17
	SymbolArray       SymbolKind = 18
)

// DocumentSymbol 文档符号（层次结构）
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           SymbolKind       `json:"kind"`
	Deprecated     bool             `json:"deprecated,omitempty"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// CodeAction 代码操作
type CodeAction struct {
	Title       string         `json:"title"`
	Kind        CodeActionKind `json:"kind,omitempty"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
	IsPreferred bool           `json:"isPreferred,omitempty"`
	Edit        *WorkspaceEdit `json:"edit,omitempty"`
	Command     *Command       `json:"command,omitempty"`
	Data        interface{}    `json:"data,omitempty"`
}

// CodeActionKind 代码操作类型
type CodeActionKind string

const (
	CodeActionQuickFix              CodeActionKind = "quickfix"
	CodeActionRefactor              CodeActionKind = "refactor"
	CodeActionRefactorExtract       CodeActionKind = "refactor.extract"
	CodeActionRefactorInline        CodeActionKind = "refactor.inline"
	CodeActionRefactorRewrite       CodeActionKind = "refactor.rewrite"
	CodeActionSource                CodeActionKind = "source"
	CodeActionSourceOrganizeImports CodeActionKind = "source.organizeImports"
)

// WorkspaceEdit 工作区编辑
type WorkspaceEdit struct {
	Changes         map[string][]TextEdit `json:"changes,omitempty"`
	DocumentChanges []TextDocumentEdit    `json:"documentChanges,omitempty"`
}

// TextDocumentEdit 文档编辑
type TextDocumentEdit struct {
	TextDocument VersionedTextDocumentIdentifier `json:"textDocument"`
	Edits        []TextEdit                      `json:"edits"`
}

// VersionedTextDocumentIdentifier 版本化文档标识
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

// Command 命令
type Command struct {
	Title     string        `json:"title"`
	Command   string        `json:"command"`
	Arguments []interface{} `json:"arguments,omitempty"`
}

// SignatureHelp 签名帮助
type SignatureHelp struct {
	Signatures      []SignatureInformation `json:"signatures"`
	ActiveSignature int                    `json:"activeSignature,omitempty"`
	ActiveParameter int                    `json:"activeParameter,omitempty"`
}

// SignatureInformation 签名信息
type SignatureInformation struct {
	Label           string                 `json:"label"`
	Documentation   *MarkupContent         `json:"documentation,omitempty"`
	Parameters      []ParameterInformation `json:"parameters,omitempty"`
	ActiveParameter int                    `json:"activeParameter,omitempty"`
}

// ParameterInformation 参数信息
type ParameterInformation struct {
	Label         string         `json:"label"`
	Documentation *MarkupContent `json:"documentation,omitempty"`
}

// ReferenceContext 引用上下文
type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// DocumentHighlight 文档高亮
type DocumentHighlight struct {
	Range Range                 `json:"range"`
	Kind  DocumentHighlightKind `json:"kind,omitempty"`
}

// DocumentHighlightKind 高亮类型
type DocumentHighlightKind int

const (
	HighlightText  DocumentHighlightKind = 1
	HighlightRead  DocumentHighlightKind = 2
	HighlightWrite DocumentHighlightKind = 3
)
