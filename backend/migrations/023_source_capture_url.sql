-- a report's source tooltip finds the screenshot of a page by its address (GET /v1/sources/screenshot)
CREATE INDEX IF NOT EXISTS source_captures_url ON source_captures(url, created_at);
