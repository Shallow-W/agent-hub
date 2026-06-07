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

