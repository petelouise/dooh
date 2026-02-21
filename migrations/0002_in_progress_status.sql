PRAGMA foreign_keys = OFF;

CREATE TABLE IF NOT EXISTS tasks_new (
  id TEXT PRIMARY KEY,
  short_id TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','in_progress','completed','archived')),
  priority TEXT NOT NULL DEFAULT 'later' CHECK(priority IN ('now','soon','later')),
  due_at TEXT,
  scheduled_at TEXT,
  rollover_enabled INTEGER NOT NULL DEFAULT 0 CHECK(rollover_enabled IN (0,1)),
  skip_weekends INTEGER NOT NULL DEFAULT 0 CHECK(skip_weekends IN (0,1)),
  estimated_minutes INTEGER,
  started_at TEXT,
  completed_at TEXT,
  archived_at TEXT,
  deleted_at TEXT,
  created_by TEXT NOT NULL REFERENCES users(id),
  updated_by TEXT NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  version INTEGER NOT NULL DEFAULT 1
);

INSERT INTO tasks_new
  SELECT id, short_id, title, description, status, priority,
         due_at, scheduled_at, rollover_enabled, skip_weekends,
         estimated_minutes, NULL, completed_at, archived_at,
         deleted_at, created_by, updated_by, created_at, updated_at, version
  FROM tasks;

DROP TABLE tasks;
ALTER TABLE tasks_new RENAME TO tasks;

PRAGMA foreign_keys = ON;
PRAGMA user_version = 2;
