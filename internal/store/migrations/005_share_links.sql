CREATE TABLE share_links (
	id TEXT PRIMARY KEY,
	inbox_id TEXT NOT NULL REFERENCES inboxes (id) ON DELETE CASCADE,
	token TEXT NOT NULL UNIQUE,
	label TEXT NOT NULL,
	created_at INTEGER NOT NULL
);

CREATE INDEX share_links_by_inbox ON share_links (inbox_id);
