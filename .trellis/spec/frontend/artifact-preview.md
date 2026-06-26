# Artifact Preview Contracts

## Scenario: PPT High-Fidelity Preview

### 1. Scope / Trigger
- Trigger: uploaded PowerPoint attachments need web preview that preserves text, formulas, and special glyphs as much as possible.
- Do not rely on `pptx-preview` alone for PPT/PPTX fidelity. Its DOM renderer is a fallback because formulas and some Office drawing features are unsupported.

### 2. Signatures
- Backend route: `GET /api/ppt-preview/*filepath`
- Frontend component: `PptxPreview({ fileUrl, fileName, previewUrl })`
- Attachment source field: `MessageAttachment.file_name` is the display and download name; fallback to basename of `file_path` only when empty.

### 3. Contracts
- `filepath` is relative to the upload directory and should be built from `attachment.file_path` after stripping the leading `uploads/`.
- The preview endpoint returns `application/pdf` with `Content-Disposition: inline` when conversion succeeds.
- Conversion order is LibreOffice first, then Windows PowerPoint COM fallback.
- Frontend must try `previewUrl` first and render the returned PDF in an iframe; if the request fails or returns non-PDF content, fall back to `pptx-preview` DOM rendering.

### 4. Validation & Error Matrix
- Path traversal -> `403`
- Non-PPT/PPTX extension -> `415`
- Missing source file -> `404`
- No converter installed -> non-2xx response; frontend falls back to DOM preview.
- Conversion failure -> non-2xx response; frontend falls back to DOM preview.

### 5. Good/Base/Bad Cases
- Good: PPT with equations renders through the converted PDF and shows formula text.
- Base: simple PPT renders through PDF or DOM fallback with correct slide aspect ratio.
- Bad: converter unavailable; user still sees DOM fallback or the download fallback instead of a blank preview.

### 6. Tests Required
- Backend handler test: path traversal is rejected.
- Backend handler test: non-PowerPoint files are rejected.
- Frontend build/type check: `PptxPreview` accepts optional `previewUrl` and keeps fallback rendering.
- Manual check: upload a PPT with formula text, open preview, verify the file name is visible and the formula appears in PDF mode.

### 7. Wrong vs Correct

#### Wrong
```tsx
<PptxPreview fileUrl={fileUrl} fileName={attachment.file_name} />
```

#### Correct
```tsx
<PptxPreview fileUrl={fileUrl} fileName={fileName} previewUrl={previewUrl} />
```

## Scenario: Project HTML File Preview

### 1. Scope / Trigger
- Trigger: `project` cards open `FilesDrawer`, and users need to preview `.html` / `.htm` files instead of only reading source code.
- Do not add a separate `html_preview` card type for single-file previews. `project` remains the directory-level entry point; HTML rendering is a file viewer mode.

### 2. Signatures
- Frontend component: `FilesDrawer({ agentId, workDir, open, onClose })`
- File read API: `readFile(agentId, workDir, targetPath) -> ReadResult`
- Preview helper: `isHtmlPreviewFile(filepath)`, `defaultFileViewMode(filepath)`

### 3. Contracts
- `.html` and `.htm` files open in `preview` mode by default.
- The file panel header shows a segmented control only for HTML files in normal view mode: `preview` and `source`.
- Preview mode renders the file content through `WebpageFrame` with `srcDoc`, reusing its sandbox behavior.
- Source mode keeps the existing code viewer path and language inference (`html` -> CodeMirror HTML mode).
- Markdown, code, text, binary, large-file, and git-history views must keep their existing behavior.

### 4. Validation & Error Matrix
- Binary file -> existing binary unsupported hint, no HTML preview.
- Too-large file -> existing too-large hint, no HTML preview.
- Empty or missing file content -> existing empty/read hint.
- HTML file with external relative assets -> preview may render the HTML shell only; full project-relative asset preview requires a separate static preview/session URL.

### 5. Good/Base/Bad Cases
- Good: selecting `index.html` in a project card immediately shows iframe preview and lets the user switch to source.
- Base: selecting `README.md` still renders markdown preview and does not show the HTML segmented control.
- Bad: adding a new card type for HTML files duplicates `project` directory semantics and forces agents to choose card protocol variants for a file-viewer concern.

### 6. Tests Required
- Unit test for `.html` / `.htm` detection and default view mode.
- Frontend build/type check must pass because `FilesDrawer` owns the Ant Design segmented control and `WebpageFrame` integration.

### 7. Wrong vs Correct

#### Wrong
```json
{"type":"html_preview","workDir":"/repo","path":"index.html"}
```

#### Correct
```tsx
<WebpageFrame srcDoc={fileContent.content} />
```
