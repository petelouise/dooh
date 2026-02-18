PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS app_config (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

INSERT OR IGNORE INTO app_config(key, value) VALUES ('timezone', 'America/Los_Angeles');

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','disabled')),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS api_keys (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id),
  key_prefix TEXT NOT NULL UNIQUE,
  key_hash TEXT NOT NULL,
  scopes TEXT NOT NULL,
  client_type TEXT NOT NULL CHECK(client_type IN ('human_cli','agent_cli','system')),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  revoked_at TEXT,
  last_used_at TEXT
);

CREATE TABLE IF NOT EXISTS collections (
  id TEXT PRIMARY KEY,
  short_id TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK(kind IN ('project','goal','tag','class','area','custom')),
  color_hex TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  archived_at TEXT,
  deleted_at TEXT,
  created_by TEXT NOT NULL REFERENCES users(id),
  updated_by TEXT NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  version INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  short_id TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','completed','archived')),
  priority TEXT NOT NULL DEFAULT 'later' CHECK(priority IN ('now','soon','later')),
  due_at TEXT,
  scheduled_at TEXT,
  rollover_enabled INTEGER NOT NULL DEFAULT 0 CHECK(rollover_enabled IN (0,1)),
  skip_weekends INTEGER NOT NULL DEFAULT 0 CHECK(skip_weekends IN (0,1)),
  estimated_minutes INTEGER,
  completed_at TEXT,
  archived_at TEXT,
  deleted_at TEXT,
  created_by TEXT NOT NULL REFERENCES users(id),
  updated_by TEXT NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  version INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS task_collections (
  task_id TEXT NOT NULL REFERENCES tasks(id),
  collection_id TEXT NOT NULL REFERENCES collections(id),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (task_id, collection_id)
);

CREATE TABLE IF NOT EXISTS collection_links (
  parent_collection_id TEXT NOT NULL REFERENCES collections(id),
  child_collection_id TEXT NOT NULL REFERENCES collections(id),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (parent_collection_id, child_collection_id),
  CHECK(parent_collection_id != child_collection_id)
);

CREATE TABLE IF NOT EXISTS collection_closure (
  ancestor_collection_id TEXT NOT NULL REFERENCES collections(id),
  descendant_collection_id TEXT NOT NULL REFERENCES collections(id),
  depth INTEGER NOT NULL CHECK(depth >= 0),
  PRIMARY KEY (ancestor_collection_id, descendant_collection_id)
);

CREATE TABLE IF NOT EXISTS task_assignees (
  task_id TEXT NOT NULL REFERENCES tasks(id),
  user_id TEXT NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (task_id, user_id)
);

CREATE TABLE IF NOT EXISTS task_subtasks (
  parent_task_id TEXT NOT NULL REFERENCES tasks(id),
  child_task_id TEXT NOT NULL REFERENCES tasks(id),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (parent_task_id, child_task_id),
  CHECK(parent_task_id != child_task_id)
);

CREATE TABLE IF NOT EXISTS task_dependencies (
  task_id TEXT NOT NULL REFERENCES tasks(id),
  blocked_by_task_id TEXT NOT NULL REFERENCES tasks(id),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (task_id, blocked_by_task_id),
  CHECK(task_id != blocked_by_task_id)
);

CREATE TABLE IF NOT EXISTS locks (
  resource_type TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  holder_user_id TEXT NOT NULL REFERENCES users(id),
  lease_expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (resource_type, resource_id)
);

CREATE TABLE IF NOT EXISTS events (
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  id TEXT NOT NULL UNIQUE,
  event_type TEXT NOT NULL,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  actor_user_id TEXT NOT NULL REFERENCES users(id),
  key_id TEXT REFERENCES api_keys(id),
  client_type TEXT NOT NULL CHECK(client_type IN ('human_cli','agent_cli','system')),
  payload_json TEXT NOT NULL,
  occurred_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_events_aggregate ON events(aggregate_type, aggregate_id);
CREATE INDEX IF NOT EXISTS idx_events_occurred_at ON events(occurred_at);

CREATE TABLE IF NOT EXISTS snapshots (
  id TEXT PRIMARY KEY,
  event_seq INTEGER NOT NULL,
  snapshot_json TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS outbox (
  id TEXT PRIMARY KEY,
  event_id TEXT NOT NULL REFERENCES events(id),
  topic TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','delivered','failed')),
  attempts INTEGER NOT NULL DEFAULT 0,
  available_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  delivered_at TEXT,
  last_error TEXT
);

CREATE INDEX IF NOT EXISTS idx_outbox_pending ON outbox(status, available_at);
