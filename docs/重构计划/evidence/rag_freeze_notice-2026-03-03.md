# RAG Freeze Notice (2026-03-03)

- Scope: `rag/` package and its subpackages.
- Rule: only refactor-plan related changes are allowed during freeze window.
- Excluded: unrelated feature additions and behavior expansions.
- Evidence:
  - baseline test suite locked by `go test ./rag/... -count=1`
  - baseline metric capture by `python scripts/rag_baseline_capture.py`
