PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS actions (
    action_id INTEGER PRIMARY KEY AUTOINCREMENT,
    action_type TEXT NOT NULL CHECK(action_type IN ('delete','move')),
    status INTEGER NOT NULL CHECK(status BETWEEN 1 AND 6),
    reason TEXT NOT NULL,
    requester TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    resolved_at TEXT NOT NULL DEFAULT '',
    review_rule TEXT NOT NULL DEFAULT '',
    decision TEXT NOT NULL DEFAULT '',
    failure TEXT NOT NULL DEFAULT '',
    operation TEXT NOT NULL DEFAULT '',
    in_progress INTEGER NOT NULL DEFAULT 0,
    blocker_id INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS deleted (
    action_id INTEGER PRIMARY KEY REFERENCES actions(action_id) ON DELETE CASCADE,
    source_path TEXT NOT NULL,
    trash_id TEXT NOT NULL UNIQUE,
    trash_path TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS moved (
    action_id INTEGER PRIMARY KEY REFERENCES actions(action_id) ON DELETE CASCADE,
    source_path TEXT NOT NULL,
    destination_path TEXT NOT NULL
);
