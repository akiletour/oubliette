CREATE TABLE inboxes (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	smtp_username TEXT NOT NULL UNIQUE,
	smtp_password TEXT NOT NULL,
	created_at INTEGER NOT NULL
);

CREATE TABLE messages (
	id TEXT PRIMARY KEY,
	inbox_id TEXT NOT NULL REFERENCES inboxes (id) ON DELETE CASCADE,
	received_at INTEGER NOT NULL,
	envelope_sender TEXT NOT NULL,
	envelope_recipients TEXT NOT NULL,
	subject TEXT NOT NULL,
	sender TEXT NOT NULL,
	recipients TEXT NOT NULL,
	size INTEGER NOT NULL,
	raw BLOB NOT NULL
);

CREATE INDEX messages_by_inbox ON messages (inbox_id, received_at DESC);
