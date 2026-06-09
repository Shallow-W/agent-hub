import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import {
  buildKnowledgePreviewModel,
  syncSelectedKnowledgeFile,
} from '../src/components/knowledge/knowledgePreviewState.mjs';

describe('knowledge preview shared state helpers', () => {
  it('renders server-extracted text even when the selected file snapshot is still binary', () => {
    const model = buildKnowledgePreviewModel({
      file: {
        id: 'file-1',
        filename: 'guide.docx',
        mime_type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
        preview_type: 'binary',
      },
      textContent: 'extracted guide',
    });

    assert.equal(model.kind, 'text');
    assert.equal(model.text, 'extracted guide');
    assert.equal(model.icon, 'text');
  });

  it('syncs selected knowledge file with refreshed list entries and clears deleted files', () => {
    const selected = {
      id: 'file-1',
      filename: 'guide.docx',
      mime_type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      preview_type: 'binary',
    };
    const refreshed = {
      ...selected,
      preview_type: 'text',
      preview_text: 'fresh extracted guide',
    };

    assert.deepEqual(syncSelectedKnowledgeFile({
      selectedFile: selected,
      selectedKbId: 'kb-1',
      knowledgeBases: [{ id: 'kb-1', files: [refreshed] }],
    }), { selectedFile: refreshed, selectedKbId: 'kb-1' });

    assert.deepEqual(syncSelectedKnowledgeFile({
      selectedFile: refreshed,
      selectedKbId: 'kb-1',
      knowledgeBases: [{ id: 'kb-1', files: [] }],
    }), { selectedFile: null, selectedKbId: null });
  });
});
