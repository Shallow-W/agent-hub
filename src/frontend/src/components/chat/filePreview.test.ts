import { describe, expect, it } from 'vitest';
import { defaultFileViewMode, isHtmlPreviewFile } from './filePreview';

describe('filePreview', () => {
  it('detects html and htm files case-insensitively', () => {
    expect(isHtmlPreviewFile('index.html')).toBe(true);
    expect(isHtmlPreviewFile('/tmp/site/About.HTM')).toBe(true);
    expect(isHtmlPreviewFile('src/App.tsx')).toBe(false);
  });

  it('opens html files in preview mode by default', () => {
    expect(defaultFileViewMode('index.html')).toBe('preview');
    expect(defaultFileViewMode('README.md')).toBe('source');
  });
});
