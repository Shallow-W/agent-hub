-- 附件排序：保证查询时按插入顺序返回
ALTER TABLE message_attachments ADD COLUMN IF NOT EXISTS sort_order integer NOT NULL DEFAULT 0;
