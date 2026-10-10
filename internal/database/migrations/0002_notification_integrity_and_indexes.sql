-- Migration 0002: notification data integrity + indexes
--
-- Context: `sendNotificationCard` used to insert a `notifications` row even
-- when the Telegram `bot.Send()` call failed, leaving `message_id = 0` rows
-- that could never be looked up or acted on again (fixed in
-- internal/app/notifications.go). Separately, `GetNotification` /
-- `UpdateNotification` / `DeleteNotification` used to query by `message_id`
-- or `notif_id` alone. Telegram message IDs are only unique *within a single
-- chat*, and `notif_id` is shared across every user watching the same Jira
-- issue/event (confirmed in the live dump: notif_id=66755 appears for both
-- user_id=131 and user_id=140) - so those lookups could read, mutate, or
-- delete a *different user's* notification row. The Go code now scopes every
-- lookup by (user_id, message_id) or (user_id, notif_id); this migration
-- cleans up the bad historical rows and adds the constraints/indexes that
-- scoping relies on.
--
-- Safe to run against the live database. Review the charset section at the
-- bottom before deciding whether to run it - it is NOT applied automatically.

START TRANSACTION;

-- 1. Remove orphaned rows created by failed Telegram sends. These rows have
--    message_id = 0 (the Go zero value) and can never be matched by any
--    real Telegram callback, so they are pure dead weight.
DELETE FROM notifications WHERE message_id = 0;

-- 2. Defensively remove any duplicate (user_id, message_id) pairs before
--    adding the unique key below, keeping the most recently inserted row.
--    (Not expected to find any beyond the message_id=0 rows already removed,
--    but guards against the ALTER TABLE below failing on unknown history.)
DELETE n1 FROM notifications n1
INNER JOIN notifications n2
  ON n1.user_id = n2.user_id
 AND n1.message_id = n2.message_id
 AND n1.id < n2.id;

-- 3. Enforce the (user_id, message_id) scoping the fixed repository code now
--    relies on, and index the (user_id, notif_id) lookup used by
--    DeleteNotification.
ALTER TABLE notifications
  ADD UNIQUE KEY uq_notifications_user_message (user_id, message_id),
  ADD INDEX idx_notifications_user_notif (user_id, notif_id);

-- 4. attach_group_projects is scanned by user_id in GetAttachGroups on every
--    notification poll cycle (every notificationPollInterval, for every
--    active user) and has no index on that column today.
ALTER TABLE attach_group_projects
  ADD INDEX idx_attach_group_projects_user_id (user_id);

-- 5. bot_settings: the running app already creates this table via
--    `CREATE TABLE IF NOT EXISTS` on every startup (internal/database/init_db.go),
--    so this is a no-op once the fixed binary has run against this database.
--    Included here so the migration file is a complete, standalone record of
--    schema state.
CREATE TABLE IF NOT EXISTS bot_settings (
	id BIGINT(20) NOT NULL AUTO_INCREMENT,
	bot_token TEXT NOT NULL,
	is_active TINYINT(1) DEFAULT 1,
	inactive_message TEXT DEFAULT NULL,
	main_admin_id BIGINT(20) DEFAULT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

COMMIT;

-- ---------------------------------------------------------------------------
-- NOT applied automatically - read before running.
--
-- The live `notifications` table's `text`/`textSummary` columns are declared
-- utf8mb3 in the dump, while internal/database/init_db.go declares them
-- utf8mb4. `CREATE TABLE IF NOT EXISTS` never alters an already-existing
-- table, so the live table never picked up that change. This is the most
-- likely explanation for the Persian-text mojibake visible throughout the
-- dump (e.g. "Ø³ÙØ§Ù" where the source text was "سلام") - the classic symptom
-- of UTF-8 bytes being written through, or read back via, a connection or
-- column that isn't actually utf8mb4.
--
--   ALTER TABLE notifications
--     CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
--
-- Running the ALTER above fixes the column definition going forward, but it
-- will NOT un-corrupt text that is already mojibake'd - that requires
-- inspecting the actual stored bytes per row and deciding whether they're
-- recoverable (e.g. by re-interpreting the bytes as latin1 and re-encoding as
-- utf8mb4) or already unrecoverably lost. Spot-check a sample of rows before
-- running any automated fix for this; it is intentionally left out of the
-- transaction above.
