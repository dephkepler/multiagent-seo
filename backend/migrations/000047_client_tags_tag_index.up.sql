-- client_tags' only index was the (client_id, tag) primary key, useless for
-- looking up by tag alone. Every tag-def rename (ON UPDATE CASCADE) or
-- delete (ON DELETE CASCADE) has to find the affected client_tags rows by
-- tag, which was a full table scan without this.
CREATE INDEX IF NOT EXISTS idx_client_tags_tag ON client_tags (tag);
