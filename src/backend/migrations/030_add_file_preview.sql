-- 知识库文件预提取：上传时预处理文件内容，便于 Agent 快速引用
ALTER TABLE knowledge_files ADD COLUMN IF NOT EXISTS preview_text TEXT DEFAULT '';
ALTER TABLE knowledge_files ADD COLUMN IF NOT EXISTS preview_type VARCHAR(20) DEFAULT 'binary';
-- preview_type 取值: 'text'(文本内容已提取), 'image'(图片), 'binary'(二进制文件), 'too_large'(超大文本文件)
