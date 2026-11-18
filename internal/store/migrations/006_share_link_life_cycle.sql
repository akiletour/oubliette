ALTER TABLE share_links ADD COLUMN expires_at INTEGER;
ALTER TABLE share_links ADD COLUMN revoked_at INTEGER;
ALTER TABLE share_links ADD COLUMN last_opened_at INTEGER;
