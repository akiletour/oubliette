CREATE TABLE first_inbox (created_at INTEGER NOT NULL);

INSERT INTO first_inbox (created_at) SELECT MIN(created_at) FROM inboxes HAVING COUNT(*) > 0;
