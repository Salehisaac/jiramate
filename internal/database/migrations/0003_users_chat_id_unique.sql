-- Migration 0003: enforce one users row per chat_id
--
-- Bug: /setToken in a group chat sent "please enter your token" successfully,
-- but on the very next message the bot logged `Unknown command: ` and the
-- group never appeared in `users` at all. Root cause: HandleSetToken tried to
-- record "this group is waiting for a token" by calling UpdateUser on the
-- `user` value the router had already looked up for this chat - but for a
-- group's first-ever /setToken, that lookup fails (no row exists yet), so the
-- router falls back to a zero-value user (id = 0). UpdateUser then silently
-- ran `UPDATE users SET stat = ? WHERE id = 0`, matching zero rows. The
-- group's state was never actually persisted, and no row was ever inserted
-- for it either - so the next message (the pasted token) couldn't be
-- recognized as "waiting for chat token" and fell through to
-- `Unknown command:`.
--
-- Fix (already applied in code): HandleSetToken now calls the new
-- UpsertGroupChatState, which does `INSERT ... ON DUPLICATE KEY UPDATE`
-- keyed on chat_id - creating the group's row on first contact instead of
-- silently no-op'ing. That upsert only works correctly if chat_id is
-- actually unique, which it was NOT: `users` had no unique constraint on
-- chat_id, so `InsertChatIntoDatabase`'s existing `ON DUPLICATE KEY UPDATE`
-- on chat_id could never trigger either - every call was a fresh INSERT,
-- meaning any chat that went through that path more than once could already
-- have accumulated duplicate rows for the same chat_id.
--
-- This migration de-duplicates any such existing rows (keeping the oldest -
-- lowest id - row per chat_id, and repointing every other table's user_id
-- references onto it so no data is dropped) and then adds the unique
-- constraint so the app can never create new duplicates again.
--
-- Safe to run against the live database. If chat_id already has no
-- duplicates, the DELETE below affects 0 rows and this is just the ALTER.

START TRANSACTION;

CREATE TEMPORARY TABLE user_id_map AS
SELECT u.id AS old_id, keeper.id AS new_id
FROM users u
INNER JOIN (
	SELECT chat_id, MIN(id) AS id
	FROM users
	WHERE chat_id IS NOT NULL
	GROUP BY chat_id
) keeper ON keeper.chat_id = u.chat_id
WHERE u.id <> keeper.id;

-- Repoint every table that stores a users.id reference away from the
-- duplicate rows and onto the surviving (keeper) row before deleting them.
-- None of these columns are covered by a uniqueness constraint together with
-- user_id, so repointing can't introduce a new conflict - at worst it leaves
-- a harmless duplicate filter/attach row under the keeper id, which the app
-- already tolerates today (e.g. InsertFilteredUser doesn't dedupe either).
UPDATE tokens t
INNER JOIN user_id_map m ON m.old_id = t.user_id
SET t.user_id = m.new_id;

UPDATE notifications n
INNER JOIN user_id_map m ON m.old_id = n.user_id
SET n.user_id = m.new_id;

UPDATE attach_group_projects agp
INNER JOIN user_id_map m ON m.old_id = agp.user_id
SET agp.user_id = m.new_id;

UPDATE filterd_users fu
INNER JOIN user_id_map m ON m.old_id = fu.user_id
SET fu.user_id = m.new_id;

UPDATE filterd_projects fp
INNER JOIN user_id_map m ON m.old_id = fp.user_id
SET fp.user_id = m.new_id;

UPDATE filterd_tasks ft
INNER JOIN user_id_map m ON m.old_id = ft.user_id
SET ft.user_id = m.new_id;

UPDATE filterd_types fty
INNER JOIN user_id_map m ON m.old_id = fty.user_id
SET fty.user_id = m.new_id;

-- attach_groups.attach_group_id is already UNIQUE, so a duplicate user row
-- can own at most one attach_groups row; repointing is unconditionally safe.
UPDATE attach_groups ag
INNER JOIN user_id_map m ON m.old_id = ag.user_id
SET ag.user_id = m.new_id;

DELETE u FROM users u
INNER JOIN user_id_map m ON m.old_id = u.id;

DROP TEMPORARY TABLE user_id_map;

ALTER TABLE users
  ADD UNIQUE KEY uq_users_chat_id (chat_id);

COMMIT;
