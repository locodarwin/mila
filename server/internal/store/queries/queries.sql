-- One query file for both dialects. Parameters use sqlc.arg() so the SQL
-- is the same for SQLite and Postgres; keep to syntax both understand.
-- Times passed in are always UTC (see store.Now), which keeps SQLite's text
-- timestamps comparable.

-- name: GetInstanceSetting :one
SELECT value FROM instance_setting WHERE key = sqlc.arg(key);

-- name: UpsertInstanceSetting :exec
INSERT INTO instance_setting (key, value, updated_at)
VALUES (sqlc.arg(key), sqlc.arg(value), sqlc.arg(updated_at))
ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at;

-- name: InsertInstanceSettingIfAbsent :exec
INSERT INTO instance_setting (key, value, updated_at)
VALUES (sqlc.arg(key), sqlc.arg(value), sqlc.arg(updated_at))
ON CONFLICT (key) DO NOTHING;

-- name: DeleteInstanceSetting :exec
DELETE FROM instance_setting WHERE key = sqlc.arg(key);

-- ReplaceInstanceSetting changes a setting only if it still holds what was
-- read, and says whether it did: a write made from a stale read loses.
-- name: ReplaceInstanceSetting :execrows
UPDATE instance_setting SET value = sqlc.arg(value), updated_at = sqlc.arg(updated_at)
WHERE key = sqlc.arg(key) AND value = sqlc.arg(was);

-- Users

-- name: CreateUser :exec
INSERT INTO users (id, email, name, is_instance_admin, settings, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(email), sqlc.arg(name), sqlc.arg(is_instance_admin), '{}', sqlc.arg(now), sqlc.arg(now));

-- name: GetUser :one
SELECT * FROM users WHERE id = sqlc.arg(id);

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = sqlc.arg(email);

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: UpdateUserName :exec
UPDATE users SET name = sqlc.arg(name), updated_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- name: SetUserAvatar :exec
-- Null removes the picture.
UPDATE users SET avatar = sqlc.narg(avatar), updated_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- name: GetAvatarOwner :one
SELECT id FROM users WHERE avatar = sqlc.arg(avatar) AND deleted_at IS NULL;

-- name: LockUser :exec
-- Holds the user's row until the transaction ends, so a read-modify-write
-- of their settings cannot interleave with another on Postgres. SQLite
-- transactions are serialized already.
UPDATE users SET updated_at = updated_at WHERE id = sqlc.arg(id);

-- name: UpdateUserSettings :exec
UPDATE users SET settings = sqlc.arg(settings), updated_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- name: ListUsers :many
SELECT * FROM users ORDER BY id;

-- name: ListLiveUsers :many
-- Everyone with an account, deleted accounts left out, by name.
SELECT * FROM users WHERE deleted_at IS NULL ORDER BY name, id;

-- name: LockInstanceAdmins :exec
-- Holds every admin's row until the transaction ends, so two admins
-- taking each other's rights at once on Postgres cannot leave none: the
-- second waits, then counts again. SQLite transactions are serialized
-- already.
UPDATE users SET is_instance_admin = is_instance_admin WHERE is_instance_admin;

-- name: SetInstanceAdmin :exec
UPDATE users SET is_instance_admin = sqlc.arg(is_instance_admin), updated_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- Passkeys

-- name: CreatePasskey :exec
INSERT INTO passkeys (id, user_id, credential_id, credential, name, created_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(credential_id), sqlc.arg(credential), sqlc.arg(name), sqlc.arg(now));

-- name: ListPasskeys :many
SELECT * FROM passkeys WHERE user_id = sqlc.arg(user_id) ORDER BY created_at;

-- name: GetPasskeyByCredentialID :one
SELECT * FROM passkeys WHERE credential_id = sqlc.arg(credential_id);

-- name: UpdatePasskeyAfterLogin :exec
UPDATE passkeys SET credential = sqlc.arg(credential), last_used_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- name: RenamePasskey :execrows
UPDATE passkeys SET name = sqlc.arg(name) WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: DeletePasskey :execrows
DELETE FROM passkeys WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- Sessions

-- name: CreateSession :exec
INSERT INTO sessions (id, token_hash, user_id, user_agent, created_at, last_seen_at, expires_at, ip, method)
VALUES (sqlc.arg(id), sqlc.arg(token_hash), sqlc.arg(user_id), sqlc.arg(user_agent), sqlc.arg(now), sqlc.arg(now), sqlc.arg(expires_at), sqlc.arg(ip), sqlc.arg(method));

-- name: GetActiveSession :one
SELECT * FROM sessions
WHERE token_hash = sqlc.arg(token_hash) AND revoked_at IS NULL AND expires_at > sqlc.arg(now);

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = sqlc.arg(now), expires_at = sqlc.arg(expires_at), ip = sqlc.arg(ip) WHERE id = sqlc.arg(id);

-- name: ListActiveSessions :many
SELECT * FROM sessions
WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL AND expires_at > sqlc.arg(now)
ORDER BY last_seen_at DESC;

-- name: RevokeSession :execrows
UPDATE sessions SET revoked_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: GetUserSession :one
SELECT * FROM sessions WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: RevokeUserSessions :many
-- Signs a user out everywhere, returning the sessions that ended.
UPDATE sessions SET revoked_at = sqlc.arg(now)
WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL AND expires_at > sqlc.arg(now)
RETURNING id;

-- name: ListSessionActivity :many
-- Every session still kept, for the admin page's last-active times and
-- counts of signed-in devices.
SELECT user_id, last_seen_at, expires_at, revoked_at FROM sessions;

-- name: RevokeSessionByTokenHash :exec
UPDATE sessions SET revoked_at = sqlc.arg(now) WHERE token_hash = sqlc.arg(token_hash) AND revoked_at IS NULL;

-- Magic links

-- name: CreateMagicLink :exec
INSERT INTO magic_links (token_hash, email, created_at, expires_at)
VALUES (sqlc.arg(token_hash), sqlc.arg(email), sqlc.arg(now), sqlc.arg(expires_at));

-- name: CountRecentMagicLinks :one
SELECT COUNT(*) FROM magic_links WHERE email = sqlc.arg(email) AND created_at > sqlc.arg(since);

-- name: EndMagicLinks :exec
-- Ends the unused sign-in links to an address, keeping their rows for the
-- hourly count.
UPDATE magic_links SET expires_at = sqlc.arg(now)
WHERE email = sqlc.arg(email) AND used_at IS NULL AND expires_at > sqlc.arg(now);

-- name: PeekMagicLink :one
-- The address a link signs in to, without spending it.
SELECT email FROM magic_links
WHERE token_hash = sqlc.arg(token_hash) AND used_at IS NULL AND expires_at > sqlc.arg(now);

-- name: UseMagicLink :one
UPDATE magic_links SET used_at = sqlc.arg(now)
WHERE token_hash = sqlc.arg(token_hash) AND used_at IS NULL AND expires_at > sqlc.arg(now)
RETURNING email;

-- Email changes

-- name: CreateEmailChange :exec
INSERT INTO email_changes (token_hash, user_id, email, created_at, expires_at, session_id)
VALUES (sqlc.arg(token_hash), sqlc.arg(user_id), sqlc.arg(email), sqlc.arg(now), sqlc.arg(expires_at), sqlc.narg(session_id));

-- name: CountRecentEmailChanges :one
SELECT COUNT(*) FROM email_changes WHERE user_id = sqlc.arg(user_id) AND created_at > sqlc.arg(since);

-- name: EndEmailChanges :exec
-- Ends a user's open changes, when a newer one replaces them.
UPDATE email_changes SET expires_at = sqlc.arg(now)
WHERE user_id = sqlc.arg(user_id) AND used_at IS NULL AND expires_at > sqlc.arg(now);

-- name: UseEmailChange :one
UPDATE email_changes SET used_at = sqlc.arg(now)
WHERE token_hash = sqlc.arg(token_hash) AND used_at IS NULL AND expires_at > sqlc.arg(now)
RETURNING user_id, email, session_id;

-- name: UpdateUserEmail :exec
UPDATE users SET email = sqlc.arg(email), updated_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- WebAuthn ceremonies

-- name: CreateChallenge :exec
INSERT INTO webauthn_challenges (id, user_id, kind, data, expires_at)
VALUES (sqlc.arg(id), sqlc.narg(user_id), sqlc.arg(kind), sqlc.arg(data), sqlc.arg(expires_at));

-- name: TakeChallenge :one
DELETE FROM webauthn_challenges
WHERE id = sqlc.arg(id) AND kind = sqlc.arg(kind) AND expires_at > sqlc.arg(now)
RETURNING *;

-- Workspaces

-- name: CreateWorkspace :exec
INSERT INTO workspaces (id, name, slug, is_personal, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(slug), sqlc.arg(is_personal), sqlc.arg(now), sqlc.arg(now));

-- name: GetWorkspace :one
SELECT * FROM workspaces WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: ListWorkspacesForUser :many
SELECT w.id, w.name, w.slug, w.is_personal, m.role
FROM workspaces w JOIN memberships m ON m.workspace_id = w.id
WHERE m.user_id = sqlc.arg(user_id) AND w.deleted_at IS NULL
ORDER BY w.is_personal DESC, w.name;

-- name: RenameWorkspace :exec
UPDATE workspaces SET name = sqlc.arg(name), updated_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- name: DeleteWorkspace :exec
UPDATE workspaces SET deleted_at = sqlc.arg(now), updated_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- Memberships

-- name: CreateMembership :exec
INSERT INTO memberships (workspace_id, user_id, role, created_at)
VALUES (sqlc.arg(workspace_id), sqlc.arg(user_id), sqlc.arg(role), sqlc.arg(now));

-- name: GetMembership :one
SELECT * FROM memberships WHERE workspace_id = sqlc.arg(workspace_id) AND user_id = sqlc.arg(user_id);

-- name: ListMembers :many
SELECT u.id, u.email, u.name, m.role, m.created_at, u.avatar
FROM memberships m JOIN users u ON u.id = m.user_id
WHERE m.workspace_id = sqlc.arg(workspace_id)
ORDER BY m.created_at;

-- name: CountSharedWorkspaces :one
-- How many live workspaces two people are both members of. Skipping
-- deleted workspaces also skips an import that is still hidden.
SELECT COUNT(*) FROM memberships a
JOIN memberships b ON b.workspace_id = a.workspace_id
JOIN workspaces w ON w.id = a.workspace_id AND w.deleted_at IS NULL
WHERE a.user_id = sqlc.arg(owner_id) AND b.user_id = sqlc.arg(viewer_id);

-- name: ListCoMemberIDs :many
-- Everyone who shares a workspace with a user, the user included.
SELECT DISTINCT b.user_id FROM memberships a
JOIN memberships b ON b.workspace_id = a.workspace_id
WHERE a.user_id = sqlc.arg(user_id);

-- name: CountOwners :one
SELECT COUNT(*) FROM memberships WHERE workspace_id = sqlc.arg(workspace_id) AND role = 'owner';

-- name: UpdateMemberRole :exec
UPDATE memberships SET role = sqlc.arg(role) WHERE workspace_id = sqlc.arg(workspace_id) AND user_id = sqlc.arg(user_id);

-- name: DeleteMembership :exec
DELETE FROM memberships WHERE workspace_id = sqlc.arg(workspace_id) AND user_id = sqlc.arg(user_id);

-- Invites

-- name: CreateInvite :exec
INSERT INTO invites (id, workspace_id, email, role, token_hash, invited_by, created_at, expires_at)
VALUES (sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(email), sqlc.arg(role), sqlc.arg(token_hash), sqlc.arg(invited_by), sqlc.arg(now), sqlc.arg(expires_at));

-- name: ListPendingInvites :many
SELECT * FROM invites
WHERE workspace_id = sqlc.arg(workspace_id) AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > sqlc.arg(now)
ORDER BY created_at;

-- name: GetPendingInvite :one
SELECT i.id, i.workspace_id, i.email, i.role, i.expires_at, i.invited_by, w.name AS workspace_name, u.name AS invited_by_name
FROM invites i
JOIN workspaces w ON w.id = i.workspace_id
JOIN users u ON u.id = i.invited_by
WHERE i.token_hash = sqlc.arg(token_hash) AND i.accepted_at IS NULL AND i.revoked_at IS NULL
  AND i.expires_at > sqlc.arg(now) AND w.deleted_at IS NULL;

-- name: AcceptInvite :execrows
UPDATE invites SET accepted_at = sqlc.arg(now) WHERE id = sqlc.arg(id) AND accepted_at IS NULL AND revoked_at IS NULL;

-- name: ListAllPendingInvites :many
-- Every open invitation on the instance, for its admins.
SELECT i.id, i.workspace_id, i.email, i.role, i.created_at, i.expires_at,
       w.name AS workspace_name, u.name AS invited_by_name
FROM invites i
JOIN workspaces w ON w.id = i.workspace_id
JOIN users u ON u.id = i.invited_by
WHERE i.accepted_at IS NULL AND i.revoked_at IS NULL AND i.expires_at > sqlc.arg(now) AND w.deleted_at IS NULL
ORDER BY i.created_at, i.id;

-- name: RevokeAnyInvite :execrows
UPDATE invites SET revoked_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND accepted_at IS NULL AND revoked_at IS NULL;

-- name: RevokeInvite :execrows
UPDATE invites SET revoked_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND accepted_at IS NULL AND revoked_at IS NULL;

-- Sync bookkeeping. Synced rows themselves (projects, statuses, tasks) are
-- read and written by the sync package from its entity table, because
-- merging works field by field across every entity.

-- name: BumpChangeSeq :one
UPDATE workspaces SET change_seq = change_seq + 1 WHERE id = sqlc.arg(id) RETURNING change_seq;

-- name: GetChangeSeq :one
SELECT change_seq FROM workspaces WHERE id = sqlc.arg(id);

-- name: InsertChange :exec
INSERT INTO change_log (workspace_id, seq, entity, entity_id, project_id, data, actor_id, created_at)
VALUES (sqlc.arg(workspace_id), sqlc.arg(seq), sqlc.arg(entity), sqlc.arg(entity_id), sqlc.narg(project_id), sqlc.arg(data), sqlc.arg(actor_id), sqlc.arg(now));

-- name: ListChanges :many
SELECT * FROM change_log
WHERE workspace_id = sqlc.arg(workspace_id) AND seq > sqlc.arg(after)
ORDER BY seq
LIMIT CAST(sqlc.arg(max_rows) AS BIGINT);

-- name: RecordAppliedOp :execrows
INSERT INTO applied_ops (op_id, workspace_id, created_at)
VALUES (sqlc.arg(op_id), sqlc.arg(workspace_id), sqlc.arg(now))
ON CONFLICT (op_id) DO NOTHING;

-- name: GetLogFloor :one
SELECT log_floor FROM workspaces WHERE id = sqlc.arg(id);

-- name: ListAllWorkspaceIDs :many
-- Every workspace, deleted ones too: compaction covers them all.
SELECT id FROM workspaces ORDER BY id;

-- name: CompactionFloor :one
-- The last change number logged before the cutoff.
SELECT CAST(COALESCE(MAX(seq), 0) AS BIGINT) FROM change_log
WHERE workspace_id = sqlc.arg(workspace_id) AND created_at < sqlc.arg(before);

-- name: RaiseLogFloor :exec
UPDATE workspaces SET log_floor = sqlc.arg(floor)
WHERE id = sqlc.arg(id) AND log_floor < sqlc.arg(floor);

-- name: DeleteChangesThrough :execrows
DELETE FROM change_log WHERE workspace_id = sqlc.arg(workspace_id) AND seq <= sqlc.arg(seq);

-- name: DeleteOldAppliedOps :execrows
DELETE FROM applied_ops WHERE created_at < sqlc.arg(before);

-- name: InsertBatchChange :exec
INSERT INTO batch_changes (op_id, workspace_id, user_id, batch, entity, entity_id, made, before, hlc, seq, created_at)
VALUES (sqlc.arg(op_id), sqlc.arg(workspace_id), sqlc.arg(user_id), sqlc.arg(batch), sqlc.arg(entity),
        sqlc.arg(entity_id), sqlc.arg(made), sqlc.arg(before), sqlc.arg(hlc), sqlc.arg(seq), sqlc.arg(now));

-- name: ListBatchChanges :many
SELECT op_id, entity, entity_id, made, before, hlc FROM batch_changes
WHERE workspace_id = sqlc.arg(workspace_id) AND user_id = sqlc.arg(user_id) AND batch = sqlc.arg(batch)
ORDER BY seq DESC;

-- name: DeleteOldBatchChanges :execrows
DELETE FROM batch_changes WHERE created_at < sqlc.arg(before);

-- name: ListGuestAccess :many
-- Each guest of a workspace with each project they are given; a guest
-- with none comes with an empty project_id.
SELECT m.user_id, COALESCE(pa.project_id, '') AS project_id
FROM memberships m
LEFT JOIN project_access pa ON pa.user_id = m.user_id
    AND pa.project_id IN (SELECT p.id FROM projects p WHERE p.workspace_id = sqlc.arg(workspace_id))
WHERE m.workspace_id = sqlc.arg(workspace_id) AND m.role = 'guest'
ORDER BY m.user_id;

-- name: ListAccessibleProjects :many
SELECT pa.project_id FROM project_access pa
JOIN projects p ON p.id = pa.project_id
WHERE pa.user_id = sqlc.arg(user_id) AND p.workspace_id = sqlc.arg(workspace_id);

-- name: GetLiveProjectWorkspace :one
SELECT workspace_id FROM projects WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: ListProjectGuests :many
-- A workspace's guests, and whether each is given the project.
SELECT u.id, u.name, u.email, pa.created_at AS shared_at, u.avatar
FROM memberships m
JOIN users u ON u.id = m.user_id
LEFT JOIN project_access pa ON pa.user_id = m.user_id AND pa.project_id = sqlc.arg(project_id)
WHERE m.workspace_id = sqlc.arg(workspace_id) AND m.role = 'guest'
ORDER BY u.name, u.id;

-- name: GrantProjectAccess :execrows
INSERT INTO project_access (project_id, user_id, created_at)
VALUES (sqlc.arg(project_id), sqlc.arg(user_id), sqlc.arg(now))
ON CONFLICT (project_id, user_id) DO NOTHING;

-- name: RevokeProjectAccess :execrows
DELETE FROM project_access WHERE project_id = sqlc.arg(project_id) AND user_id = sqlc.arg(user_id);

-- name: DeleteMemberProjectAccess :exec
-- A guest who leaves a workspace, or becomes a member of it, takes no
-- projects of it along, should they be a guest there again.
DELETE FROM project_access
WHERE user_id = sqlc.arg(user_id)
  AND project_id IN (SELECT p.id FROM projects p WHERE p.workspace_id = sqlc.arg(workspace_id));

-- name: SetMemberResetSeq :exec
UPDATE memberships SET reset_seq = sqlc.arg(reset_seq)
WHERE workspace_id = sqlc.arg(workspace_id) AND user_id = sqlc.arg(user_id);

-- name: ListMemberIDs :many
SELECT user_id FROM memberships WHERE workspace_id = sqlc.arg(workspace_id);

-- name: ListMemberSettings :many
-- Each member's settings, to count who uses each theme the workspace offers.
SELECT u.id, u.settings FROM memberships m JOIN users u ON u.id = m.user_id
WHERE m.workspace_id = sqlc.arg(workspace_id);

-- name: ListWorkspaceThemeIDs :many
SELECT id FROM workspace_themes WHERE workspace_id = sqlc.arg(workspace_id) AND deleted_at IS NULL;

-- Activity: what happened to each task, kept forever.

-- name: InsertActivity :exec
INSERT INTO activity (id, workspace_id, task_id, actor_id, entity, entity_id, kind, data, created_at)
VALUES (sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(task_id), sqlc.arg(actor_id), sqlc.arg(entity), sqlc.arg(entity_id), sqlc.arg(kind), sqlc.arg(data), sqlc.arg(now));

-- name: LatestTaskActivity :one
-- A task's newest history entry, so an edit to its notes in the same
-- editing session updates it rather than adding another.
SELECT id, actor_id, entity, kind, data, created_at
FROM activity WHERE workspace_id = sqlc.arg(workspace_id) AND task_id = sqlc.arg(task_id)
ORDER BY created_at DESC, id DESC LIMIT 1;

-- name: UpdateActivityData :exec
UPDATE activity SET data = sqlc.arg(data), created_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- name: ListActivity :many
SELECT a.id, a.actor_id, COALESCE(u.name, '') AS actor_name, a.entity, a.entity_id, a.kind, a.data, a.created_at
FROM activity a LEFT JOIN users u ON u.id = a.actor_id
WHERE a.workspace_id = sqlc.arg(workspace_id) AND a.task_id = sqlc.arg(task_id)
ORDER BY a.created_at DESC, a.id DESC
LIMIT CAST(sqlc.arg(max_rows) AS BIGINT);

-- name: ListStorageKeys :many
SELECT storage_key FROM attachments
UNION SELECT storage_key FROM page_attachments
UNION SELECT avatar FROM users WHERE avatar IS NOT NULL;

-- Jobs: work done outside a request, claimed by the runner.

-- name: InsertJob :exec
INSERT INTO jobs (id, kind, payload, run_at, attempts, created_at)
VALUES (sqlc.arg(id), sqlc.arg(kind), sqlc.arg(payload), sqlc.arg(run_at), 0, sqlc.arg(now));

-- name: ListDueJobs :many
-- A page of due jobs after the one at (after_run_at, after_id), so a pass
-- can walk past jobs it leaves to others.
SELECT id, kind, run_at FROM jobs
WHERE done_at IS NULL AND attempts < sqlc.arg(max_attempts) AND run_at <= sqlc.arg(now)
  AND (locked_until IS NULL OR locked_until < sqlc.arg(now))
  AND (run_at > sqlc.arg(after_run_at) OR (run_at = sqlc.arg(after_run_at) AND id > sqlc.arg(after_id)))
ORDER BY run_at, id
LIMIT CAST(sqlc.arg(max_rows) AS BIGINT);

-- name: ListDueJobsOfKind :many
SELECT id FROM jobs
WHERE kind = sqlc.arg(kind) AND done_at IS NULL AND attempts < sqlc.arg(max_attempts) AND run_at <= sqlc.arg(now)
  AND (locked_until IS NULL OR locked_until < sqlc.arg(now))
ORDER BY run_at, id
LIMIT CAST(sqlc.arg(max_rows) AS BIGINT);

-- name: ClaimJob :one
UPDATE jobs SET locked_until = sqlc.arg(locked_until), attempts = attempts + 1
WHERE id = sqlc.arg(id) AND done_at IS NULL AND attempts < sqlc.arg(max_attempts)
  AND (locked_until IS NULL OR locked_until < sqlc.arg(now))
RETURNING *;

-- name: FinishJob :exec
UPDATE jobs SET done_at = sqlc.arg(now), locked_until = NULL, last_error = NULL WHERE id = sqlc.arg(id);

-- name: RetryJob :exec
UPDATE jobs SET run_at = sqlc.arg(run_at), last_error = sqlc.arg(last_error), locked_until = NULL WHERE id = sqlc.arg(id);

-- name: GetJob :one
SELECT * FROM jobs WHERE id = sqlc.arg(id);

-- name: DeleteOldJobs :execrows
DELETE FROM jobs
WHERE (done_at IS NOT NULL AND done_at < sqlc.arg(done_before))
   OR (done_at IS NULL AND attempts >= sqlc.arg(max_attempts) AND run_at < sqlc.arg(failed_before));

-- Notifications
--
-- What a user reads back counts only where they can still see it: they are
-- still a member, and a guest still has the task's project shared with them
-- (the rule in syncer's viewer.sees). Access lost later hides the rest.
-- Nor does one about something removed: a deleted workspace, task or project.
-- These are hidden and not deleted, so restoring it brings them back.

-- name: InsertNotification :exec
INSERT INTO notifications (id, user_id, workspace_id, kind, task_id, page_thread_id, actor_id, title, body, in_app, created_at, batch)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(workspace_id), sqlc.arg(kind), sqlc.narg(task_id), sqlc.narg(page_thread_id), sqlc.narg(actor_id),
        sqlc.arg(title), sqlc.arg(body), sqlc.arg(in_app), sqlc.arg(now), sqlc.narg(batch));

-- name: ListBatchToPush :many
-- A person's notifications of one kind from one change to many tasks, not pushed yet.
SELECT * FROM notifications
WHERE user_id = sqlc.arg(user_id) AND batch = sqlc.arg(batch) AND kind = sqlc.arg(kind) AND pushed_at IS NULL
ORDER BY id;

-- name: GetNotification :one
SELECT * FROM notifications WHERE id = sqlc.arg(id);

-- name: ListNotifications :many
SELECT n.id, n.workspace_id, n.kind, n.task_id, n.page_thread_id, n.actor_id, COALESCE(u.name, '') AS actor_name,
       n.title, n.body, n.created_at, n.read_at, u.avatar AS actor_avatar
FROM notifications n LEFT JOIN users u ON u.id = n.actor_id
WHERE n.user_id = sqlc.arg(user_id) AND n.in_app = TRUE
  AND (n.created_at < sqlc.arg(before_at) OR (n.created_at = sqlc.arg(before_at) AND n.id < sqlc.arg(before_id)))
  AND EXISTS (SELECT 1 FROM memberships m WHERE m.workspace_id = n.workspace_id AND m.user_id = n.user_id
              AND (m.role <> 'guest' OR (n.task_id IS NULL AND n.page_thread_id IS NULL) OR EXISTS (
                SELECT 1 FROM tasks t JOIN project_access pa ON pa.project_id = t.project_id
                WHERE t.id = n.task_id AND pa.user_id = n.user_id) OR EXISTS (
                SELECT 1 FROM page_threads th JOIN pages g ON g.id = th.page_id
                JOIN project_access pa ON pa.project_id = g.project_id
                WHERE th.id = n.page_thread_id AND pa.user_id = n.user_id)))
  AND EXISTS (SELECT 1 FROM workspaces w WHERE w.id = n.workspace_id AND w.deleted_at IS NULL)
  AND (n.task_id IS NULL OR NOT EXISTS (
    SELECT 1 FROM tasks t LEFT JOIN projects p ON p.id = t.project_id
    WHERE t.id = n.task_id AND (t.deleted_at IS NOT NULL OR p.deleted_at IS NOT NULL)))
  AND (n.page_thread_id IS NULL OR NOT EXISTS (
    SELECT 1 FROM page_threads th JOIN pages g ON g.id = th.page_id LEFT JOIN projects p ON p.id = g.project_id
    WHERE th.id = n.page_thread_id AND (th.deleted_at IS NOT NULL OR g.deleted_at IS NOT NULL OR p.deleted_at IS NOT NULL)))
ORDER BY n.created_at DESC, n.id DESC
LIMIT CAST(sqlc.arg(max_rows) AS BIGINT);

-- name: CountUnreadNotifications :one
SELECT COUNT(*) FROM notifications n
WHERE n.user_id = sqlc.arg(user_id) AND n.in_app = TRUE AND n.read_at IS NULL
  AND EXISTS (SELECT 1 FROM memberships m WHERE m.workspace_id = n.workspace_id AND m.user_id = n.user_id
              AND (m.role <> 'guest' OR (n.task_id IS NULL AND n.page_thread_id IS NULL) OR EXISTS (
                SELECT 1 FROM tasks t JOIN project_access pa ON pa.project_id = t.project_id
                WHERE t.id = n.task_id AND pa.user_id = n.user_id) OR EXISTS (
                SELECT 1 FROM page_threads th JOIN pages g ON g.id = th.page_id
                JOIN project_access pa ON pa.project_id = g.project_id
                WHERE th.id = n.page_thread_id AND pa.user_id = n.user_id)))
  AND EXISTS (SELECT 1 FROM workspaces w WHERE w.id = n.workspace_id AND w.deleted_at IS NULL)
  AND (n.task_id IS NULL OR NOT EXISTS (
    SELECT 1 FROM tasks t LEFT JOIN projects p ON p.id = t.project_id
    WHERE t.id = n.task_id AND (t.deleted_at IS NOT NULL OR p.deleted_at IS NOT NULL)))
  AND (n.page_thread_id IS NULL OR NOT EXISTS (
    SELECT 1 FROM page_threads th JOIN pages g ON g.id = th.page_id LEFT JOIN projects p ON p.id = g.project_id
    WHERE th.id = n.page_thread_id AND (th.deleted_at IS NOT NULL OR g.deleted_at IS NOT NULL OR p.deleted_at IS NOT NULL)));

-- name: ListUnreadNotificationsSince :many
SELECT n.* FROM notifications n
WHERE n.user_id = sqlc.arg(user_id) AND n.in_app = TRUE AND n.read_at IS NULL AND n.created_at > sqlc.arg(since)
  AND EXISTS (SELECT 1 FROM memberships m WHERE m.workspace_id = n.workspace_id AND m.user_id = n.user_id
              AND (m.role <> 'guest' OR (n.task_id IS NULL AND n.page_thread_id IS NULL) OR EXISTS (
                SELECT 1 FROM tasks t JOIN project_access pa ON pa.project_id = t.project_id
                WHERE t.id = n.task_id AND pa.user_id = n.user_id) OR EXISTS (
                SELECT 1 FROM page_threads th JOIN pages g ON g.id = th.page_id
                JOIN project_access pa ON pa.project_id = g.project_id
                WHERE th.id = n.page_thread_id AND pa.user_id = n.user_id)))
  AND EXISTS (SELECT 1 FROM workspaces w WHERE w.id = n.workspace_id AND w.deleted_at IS NULL)
  AND (n.task_id IS NULL OR NOT EXISTS (
    SELECT 1 FROM tasks t LEFT JOIN projects p ON p.id = t.project_id
    WHERE t.id = n.task_id AND (t.deleted_at IS NOT NULL OR p.deleted_at IS NOT NULL)))
  AND (n.page_thread_id IS NULL OR NOT EXISTS (
    SELECT 1 FROM page_threads th JOIN pages g ON g.id = th.page_id LEFT JOIN projects p ON p.id = g.project_id
    WHERE th.id = n.page_thread_id AND (th.deleted_at IS NOT NULL OR g.deleted_at IS NOT NULL OR p.deleted_at IS NOT NULL)))
ORDER BY n.created_at, n.id;

-- name: ListUsersNotifiedAbout :many
-- Who holds a notification about a task or a page's thread, or about one in a project.
SELECT DISTINCT n.user_id FROM notifications n JOIN tasks t ON t.id = n.task_id
WHERE t.id = sqlc.arg(id) OR t.project_id = sqlc.arg(id)
UNION
SELECT DISTINCT n.user_id FROM notifications n JOIN page_threads th ON th.id = n.page_thread_id JOIN pages g ON g.id = th.page_id
WHERE g.id = sqlc.arg(id) OR g.project_id = sqlc.arg(id);

-- name: MarkNotificationRead :execrows
UPDATE notifications SET read_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND read_at IS NULL;

-- name: MarkAllNotificationsRead :exec
-- Marks those up to and including a cursor (the newest the user has seen),
-- so one that arrived meanwhile stays unread.
UPDATE notifications SET read_at = sqlc.arg(now)
WHERE user_id = sqlc.arg(user_id) AND read_at IS NULL
  AND (created_at < sqlc.arg(through_at) OR (created_at = sqlc.arg(through_at) AND id <= sqlc.arg(through_id)));

-- name: ClearNotification :exec
DELETE FROM notifications WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: ClearNotificationsThrough :exec
-- Clears those up to and including a cursor (the newest the user has seen),
-- so one that arrived meanwhile stays.
DELETE FROM notifications
WHERE user_id = sqlc.arg(user_id)
  AND (created_at < sqlc.arg(through_at) OR (created_at = sqlc.arg(through_at) AND id <= sqlc.arg(through_id)));

-- name: SetNotificationEmailed :exec
UPDATE notifications SET emailed_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- name: SetNotificationPushed :exec
UPDATE notifications SET pushed_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- name: GetDigest :one
SELECT * FROM digests WHERE user_id = sqlc.arg(user_id);

-- name: UpsertDigest :exec
INSERT INTO digests (user_id, last_date, sent_at) VALUES (sqlc.arg(user_id), sqlc.arg(last_date), sqlc.arg(now))
ON CONFLICT (user_id) DO UPDATE SET last_date = excluded.last_date, sent_at = excluded.sent_at;

-- name: ClaimWeeklyReport :execrows
-- The weekly report for a week, claimed inside the transaction that queues
-- it: 1 the first time, 0 once that week (or a later one) is recorded.
INSERT INTO weekly_reports (user_id, workspace_id, week, sent_at)
VALUES (sqlc.arg(user_id), sqlc.arg(workspace_id), sqlc.arg(week), sqlc.arg(now))
ON CONFLICT (user_id, workspace_id) DO UPDATE SET week = excluded.week, sent_at = excluded.sent_at
WHERE weekly_reports.week < excluded.week;

-- name: GetWeeklyReport :one
SELECT * FROM weekly_reports WHERE user_id = sqlc.arg(user_id) AND workspace_id = sqlc.arg(workspace_id);

-- Reminders and due notices: what the scan has already sent. Each claim
-- reports 1 the first time and 0 after, inside the transaction that
-- creates the notification, so each goes out once.

-- name: ClaimReminderDelivery :execrows
INSERT INTO reminder_deliveries (reminder_id, remind_at, sent_at)
VALUES (sqlc.arg(reminder_id), sqlc.arg(remind_at), sqlc.arg(now))
ON CONFLICT (reminder_id, remind_at) DO NOTHING;

-- name: ClaimDueNotice :execrows
INSERT INTO due_notices (task_id, user_id, due, sent_at)
VALUES (sqlc.arg(task_id), sqlc.arg(user_id), sqlc.arg(due), sqlc.arg(now))
ON CONFLICT (task_id, user_id, due) DO NOTHING;

-- name: ReminderDelivered :one
SELECT COUNT(*) FROM reminder_deliveries WHERE reminder_id = sqlc.arg(reminder_id) AND remind_at = sqlc.arg(remind_at);

-- name: DueNoticeSent :one
SELECT COUNT(*) FROM due_notices WHERE task_id = sqlc.arg(task_id) AND user_id = sqlc.arg(user_id) AND due = sqlc.arg(due);

-- Web push subscriptions. A subscription is live while the session that
-- made it is.

-- name: UpsertPushSubscription :exec
-- The same browser subscribing again (on another account, or with new
-- keys) takes over its row.
INSERT INTO push_subscriptions (id, user_id, session_id, endpoint, p256dh, auth, user_agent, created_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(session_id), sqlc.arg(endpoint), sqlc.arg(p256dh), sqlc.arg(auth), sqlc.arg(user_agent), sqlc.arg(now))
ON CONFLICT (endpoint) DO UPDATE SET user_id = excluded.user_id, session_id = excluded.session_id,
    p256dh = excluded.p256dh, auth = excluded.auth, user_agent = excluded.user_agent, warned_at = NULL;

-- name: TrimPushSubscriptions :exec
-- Keeps a user's most recently used subscriptions, up to keep.
DELETE FROM push_subscriptions WHERE push_subscriptions.user_id = sqlc.arg(owner_id) AND push_subscriptions.id NOT IN (
    SELECT p.id FROM push_subscriptions p WHERE p.user_id = sqlc.arg(owner_id)
    ORDER BY COALESCE(p.last_used_at, p.created_at) DESC, p.id DESC LIMIT CAST(sqlc.arg(keep) AS BIGINT)
);

-- name: ListLivePushSubscriptions :many
SELECT p.* FROM push_subscriptions p JOIN sessions s ON s.id = p.session_id
WHERE p.user_id = sqlc.arg(owner_id) AND s.revoked_at IS NULL AND s.expires_at > sqlc.arg(now)
ORDER BY p.created_at, p.id;

-- name: GetLivePushSubscription :one
SELECT p.* FROM push_subscriptions p JOIN sessions s ON s.id = p.session_id
WHERE p.id = sqlc.arg(id) AND s.revoked_at IS NULL AND s.expires_at > sqlc.arg(now);

-- name: TouchPushSubscription :exec
UPDATE push_subscriptions SET last_used_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- name: DeletePushSubscription :exec
DELETE FROM push_subscriptions WHERE id = sqlc.arg(id);

-- name: DeleteUserPushSubscription :exec
DELETE FROM push_subscriptions WHERE user_id = sqlc.arg(user_id) AND endpoint = sqlc.arg(endpoint);

-- name: ListLapsingPushSubscriptions :many
-- Browsers whose session ends between now and soon, with what decides
-- whether they are told and how.
SELECT p.id, p.user_id, p.session_id, p.warned_at, s.created_at AS session_created_at, s.expires_at AS session_expires_at
FROM push_subscriptions p JOIN sessions s ON s.id = p.session_id
WHERE s.revoked_at IS NULL AND s.expires_at > sqlc.arg(now) AND s.expires_at <= sqlc.arg(soon)
ORDER BY p.id;

-- name: SetPushSubscriptionWarned :exec
UPDATE push_subscriptions SET warned_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- name: DeleteDeadPushSubscriptions :execrows
-- Subscriptions whose session has ended.
DELETE FROM push_subscriptions WHERE session_id IN (
    SELECT s.id FROM sessions s WHERE s.revoked_at IS NOT NULL OR s.expires_at <= sqlc.arg(now)
);

-- Calendar feeds

-- name: InsertIcalToken :exec
INSERT INTO ical_tokens (id, user_id, workspace_id, scope, target_id, token_hash, name, created_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(workspace_id), sqlc.arg(scope), sqlc.arg(target_id), sqlc.arg(token_hash), sqlc.arg(name), sqlc.arg(now));

-- name: ListIcalTokens :many
-- A user's live feeds in a workspace, oldest first.
SELECT * FROM ical_tokens
WHERE user_id = sqlc.arg(user_id) AND workspace_id = sqlc.arg(workspace_id) AND revoked_at IS NULL
ORDER BY created_at, id;

-- name: CountIcalTokens :one
SELECT COUNT(*) FROM ical_tokens WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: GetIcalTokenByHash :one
SELECT * FROM ical_tokens WHERE token_hash = sqlc.arg(token_hash) AND revoked_at IS NULL;

-- name: RevokeIcalToken :execrows
UPDATE ical_tokens SET revoked_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND workspace_id = sqlc.arg(workspace_id) AND revoked_at IS NULL;

-- name: RevokeMemberIcalTokens :exec
-- Ends every feed someone made in a workspace they leave, so the links do
-- not come back to life if they join it again.
UPDATE ical_tokens SET revoked_at = sqlc.arg(now)
WHERE user_id = sqlc.arg(user_id) AND workspace_id = sqlc.arg(workspace_id) AND revoked_at IS NULL;

-- name: RevokeUserIcalTokens :exec
-- Ends every feed someone made, in every workspace, when an admin signs
-- them out everywhere: a feed outlives sessions, so a stolen one would.
UPDATE ical_tokens SET revoked_at = sqlc.arg(now) WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: TouchIcalToken :exec
-- Records a fetch, at most once per interval, so calendar apps polling
-- often do not write every time.
UPDATE ical_tokens SET last_used_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND (last_used_at IS NULL OR last_used_at < sqlc.arg(before));

-- Jobs in the slow lane (exports, imports, backups), whose kinds share a
-- prefix.

-- name: ListDueJobsOfLane :many
SELECT id FROM jobs
WHERE kind LIKE sqlc.arg(pattern) AND done_at IS NULL AND attempts < sqlc.arg(max_attempts) AND run_at <= sqlc.arg(now)
  AND (locked_until IS NULL OR locked_until < sqlc.arg(now))
ORDER BY run_at, id
LIMIT CAST(sqlc.arg(max_rows) AS BIGINT);

-- Exports

-- name: InsertExport :exec
INSERT INTO exports (id, user_id, workspace_id, format, status, file_name, created_at, expires_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(workspace_id), sqlc.arg(format), 'queued', sqlc.arg(file_name), sqlc.arg(now), sqlc.arg(expires_at));

-- name: GetExport :one
SELECT * FROM exports WHERE id = sqlc.arg(id);

-- name: ListExports :many
-- A user's exports, newest first.
SELECT e.*, w.name AS workspace_name FROM exports e JOIN workspaces w ON w.id = e.workspace_id
WHERE e.user_id = sqlc.arg(user_id)
ORDER BY e.created_at DESC, e.id DESC;

-- name: CountUnfinishedExports :one
SELECT COUNT(*) FROM exports WHERE user_id = sqlc.arg(user_id) AND status IN ('queued', 'running');

-- name: CountKeptExports :one
-- A user's exports on the way or ready to download.
SELECT COUNT(*) FROM exports WHERE user_id = sqlc.arg(user_id) AND status <> 'failed' AND expires_at > sqlc.arg(now);

-- name: StartExport :exec
UPDATE exports SET status = 'running' WHERE id = sqlc.arg(id);

-- name: FinishExport :exec
UPDATE exports SET status = 'ready', size = sqlc.arg(size), error = NULL, finished_at = sqlc.arg(now), expires_at = sqlc.arg(expires_at)
WHERE id = sqlc.arg(id);

-- name: FailExport :exec
UPDATE exports SET status = 'failed', error = sqlc.arg(error), finished_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- name: DeleteExport :execrows
DELETE FROM exports WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: DeleteExpiredExports :execrows
DELETE FROM exports WHERE expires_at < sqlc.arg(now);

-- name: ListExportIDs :many
SELECT id FROM exports;

-- Imports

-- name: InsertImport :exec
INSERT INTO imports (id, user_id, file_name, keep_ids, status, created_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(file_name), sqlc.arg(keep_ids), 'queued', sqlc.arg(now));

-- name: GetImport :one
SELECT * FROM imports WHERE id = sqlc.arg(id);

-- name: ListImports :many
-- A user's imports, newest first.
SELECT i.*, COALESCE(w.name, '') AS workspace_name FROM imports i LEFT JOIN workspaces w ON w.id = i.workspace_id
WHERE i.user_id = sqlc.arg(user_id)
ORDER BY i.created_at DESC, i.id DESC;

-- name: CountUnfinishedImports :one
SELECT COUNT(*) FROM imports WHERE user_id = sqlc.arg(user_id) AND status IN ('queued', 'running');

-- name: StartImport :exec
UPDATE imports SET status = 'running', workspace_id = sqlc.narg(workspace_id) WHERE id = sqlc.arg(id);

-- name: RequeueImport :exec
-- An import the server stopped under, queued again for its retry.
UPDATE imports SET status = 'queued' WHERE id = sqlc.arg(id) AND status = 'running';

-- name: FinishImport :exec
UPDATE imports SET status = 'ready', report = sqlc.arg(report), error = NULL, finished_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- name: FailImport :exec
UPDATE imports SET status = 'failed', workspace_id = NULL, report = sqlc.arg(report), error = sqlc.arg(error), finished_at = sqlc.arg(now)
WHERE id = sqlc.arg(id);

-- name: DeleteOldImports :execrows
DELETE FROM imports WHERE finished_at IS NOT NULL AND finished_at < sqlc.arg(before);

-- name: ListImportIDs :many
SELECT id FROM imports WHERE status IN ('queued', 'running');

-- name: ShowWorkspace :exec
-- Brings an imported workspace into sight once it is complete.
UPDATE workspaces SET deleted_at = NULL, updated_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- name: PurgeWorkspace :exec
-- Removes a workspace and, by the foreign keys, everything in it.
DELETE FROM workspaces WHERE id = sqlc.arg(id);

-- name: GetAnyWorkspace :one
-- A workspace whether or not it is deleted or still being imported.
SELECT * FROM workspaces WHERE id = sqlc.arg(id);

-- Account deletion

-- name: ListUserMemberships :many
-- Every workspace a user belongs to, deleted ones too, with how many
-- members and owners each has.
SELECT w.id, w.name, w.is_personal, w.deleted_at, m.role,
       (SELECT COUNT(*) FROM memberships x WHERE x.workspace_id = w.id) AS members,
       (SELECT COUNT(*) FROM memberships x WHERE x.workspace_id = w.id AND x.role = 'owner') AS owners
FROM memberships m JOIN workspaces w ON w.id = m.workspace_id
WHERE m.user_id = sqlc.arg(user_id)
ORDER BY w.name, w.id;

-- name: CountInstanceAdmins :one
SELECT COUNT(*) FROM users WHERE is_instance_admin AND deleted_at IS NULL;

-- name: LockOwners :exec
-- Holds a workspace's owners' rows until the transaction ends, so two
-- owners leaving at once on Postgres cannot leave none: the second waits,
-- then counts again. SQLite transactions are serialized already.
UPDATE memberships SET role = role WHERE workspace_id = sqlc.arg(workspace_id) AND role = 'owner';

-- name: ListActorNotifications :many
-- The notifications someone's actions sent to others, whose text names
-- them.
SELECT id, kind, title, body FROM notifications WHERE actor_id = sqlc.arg(actor_id) AND user_id <> sqlc.arg(actor_id);

-- name: UpdateNotificationText :exec
UPDATE notifications SET title = sqlc.arg(title), body = sqlc.arg(body) WHERE id = sqlc.arg(id);

-- name: ListInstanceAdmins :many
SELECT * FROM users WHERE is_instance_admin AND deleted_at IS NULL ORDER BY id;

-- name: ListUnfinishedImportWorkspaces :many
SELECT workspace_id FROM imports
WHERE user_id = sqlc.arg(user_id) AND status IN ('queued', 'running') AND workspace_id IS NOT NULL;

-- name: AnonymizeUser :exec
UPDATE users SET name = 'Deleted user', email = sqlc.arg(email), settings = '{}', is_instance_admin = FALSE,
    avatar = NULL, deleted_at = sqlc.arg(now), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id);

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserPasskeys :exec
DELETE FROM passkeys WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserChallenges :exec
DELETE FROM webauthn_challenges WHERE user_id = sqlc.arg(user_id);

-- name: DeleteMagicLinks :exec
DELETE FROM magic_links WHERE email = sqlc.arg(email);

-- name: DeleteMemberNotifications :exec
-- What someone was told about a workspace they have left.
DELETE FROM notifications WHERE user_id = sqlc.arg(user_id) AND workspace_id = sqlc.arg(workspace_id);

-- name: DeleteGuestProjectNotifications :exec
-- What a guest was told about a project no longer shared with them.
DELETE FROM notifications WHERE user_id = sqlc.arg(user_id)
    AND (task_id IN (SELECT tasks.id FROM tasks WHERE tasks.project_id = sqlc.arg(project_id))
         OR page_thread_id IN (SELECT page_threads.id FROM page_threads WHERE page_threads.page_id IN (SELECT pages.id FROM pages WHERE pages.project_id = sqlc.arg(project_id))));

-- name: DeleteUserEmailChanges :exec
DELETE FROM email_changes WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserMemberships :exec
DELETE FROM memberships WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserProjectAccess :exec
DELETE FROM project_access WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserNotifications :exec
DELETE FROM notifications WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserDigest :exec
DELETE FROM digests WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserWeeklyReports :exec
DELETE FROM weekly_reports WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserDueNotices :exec
DELETE FROM due_notices WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserPushSubscriptions :exec
DELETE FROM push_subscriptions WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserIcalTokens :exec
DELETE FROM ical_tokens WHERE user_id = sqlc.arg(user_id);

-- name: DeleteMemberExports :exec
-- A person's exports of one workspace, as they leave it or become a guest.
DELETE FROM exports WHERE user_id = sqlc.arg(user_id) AND workspace_id = sqlc.arg(workspace_id);

-- name: DeleteUserExports :exec
DELETE FROM exports WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserImports :exec
DELETE FROM imports WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserSecurityEvents :exec
DELETE FROM security_events WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserKnownDevices :exec
DELETE FROM known_devices WHERE user_id = sqlc.arg(user_id);

-- name: ListUserThemes :many
SELECT * FROM user_themes WHERE user_id = sqlc.arg(user_id) ORDER BY created_at, id;

-- name: GetUserTheme :one
SELECT * FROM user_themes WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: CountUserThemes :one
SELECT COUNT(*) FROM user_themes WHERE user_id = sqlc.arg(user_id);

-- name: UpsertUserTheme :execrows
-- Writes a theme, keeping when it was made. Someone else's id changes
-- nothing: no rows, which the caller answers as not found.
INSERT INTO user_themes (id, user_id, name, body, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(body), sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (id) DO UPDATE SET name = excluded.name, body = excluded.body, updated_at = excluded.updated_at
WHERE user_themes.user_id = excluded.user_id;

-- name: DeleteUserTheme :execrows
DELETE FROM user_themes WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: DeleteUserThemes :exec
DELETE FROM user_themes WHERE user_id = sqlc.arg(user_id);

-- name: ListFormerMembers :many
-- People whose work is in a workspace (comments, files, tasks they made)
-- but who are no longer members: those who left, and deleted accounts.
SELECT u.id, u.name, u.deleted_at FROM users u
WHERE u.id IN (
    SELECT c.author_id FROM comments c WHERE c.workspace_id = sqlc.arg(workspace_id)
    UNION SELECT pc.author_id FROM page_comments pc WHERE pc.workspace_id = sqlc.arg(workspace_id)
    UNION SELECT pt.created_by FROM page_threads pt WHERE pt.workspace_id = sqlc.arg(workspace_id)
    UNION SELECT a.uploaded_by FROM attachments a WHERE a.workspace_id = sqlc.arg(workspace_id)
    UNION SELECT t.created_by FROM tasks t WHERE t.workspace_id = sqlc.arg(workspace_id) AND t.created_by IS NOT NULL
)
AND u.id NOT IN (SELECT m.user_id FROM memberships m WHERE m.workspace_id = sqlc.arg(workspace_id))
ORDER BY u.name, u.id;

-- Security log and known devices

-- name: InsertSecurityEvent :exec
INSERT INTO security_events (id, user_id, kind, method, new_device, detail, ip, user_agent, created_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(kind), sqlc.arg(method), sqlc.arg(new_device), sqlc.arg(detail),
        sqlc.arg(ip), sqlc.arg(user_agent), sqlc.arg(now));

-- name: ListSecurityEvents :many
-- A user's security log, newest first, a page at a time: IDs grow with
-- time, so the page after one starts below its last ID.
SELECT * FROM security_events
WHERE user_id = sqlc.arg(user_id) AND (CAST(sqlc.arg(before) AS TEXT) = '' OR id < CAST(sqlc.arg(before) AS TEXT))
ORDER BY id DESC
LIMIT CAST(sqlc.arg(max_rows) AS BIGINT);

-- name: CountKnownDevices :one
SELECT COUNT(*) FROM known_devices WHERE user_id = sqlc.arg(user_id);

-- name: TouchKnownDevice :execrows
UPDATE known_devices SET last_seen_at = sqlc.arg(now)
WHERE user_id = sqlc.arg(user_id) AND device_hash = sqlc.arg(device_hash);

-- name: InsertKnownDevice :exec
INSERT INTO known_devices (user_id, device_hash, created_at, last_seen_at)
VALUES (sqlc.arg(user_id), sqlc.arg(device_hash), sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (user_id, device_hash) DO UPDATE SET last_seen_at = excluded.last_seen_at;

-- Upkeep: sign-in tokens and records past their use, removed hourly.

-- name: DeleteEndedSessions :execrows
DELETE FROM sessions WHERE expires_at < sqlc.arg(before) OR revoked_at < sqlc.arg(before);

-- name: DeleteOldMagicLinks :execrows
DELETE FROM magic_links WHERE expires_at < sqlc.arg(before);

-- name: DeleteOldEmailChanges :execrows
DELETE FROM email_changes WHERE expires_at < sqlc.arg(before);

-- name: DeleteExpiredChallenges :execrows
DELETE FROM webauthn_challenges WHERE expires_at < sqlc.arg(before);

-- name: DeleteClosedInvites :execrows
DELETE FROM invites
WHERE expires_at < sqlc.arg(before) OR accepted_at < sqlc.arg(before) OR revoked_at < sqlc.arg(before);

-- name: DeleteOldSecurityEvents :execrows
DELETE FROM security_events WHERE created_at < sqlc.arg(before);

-- name: DeleteOldKnownDevices :execrows
DELETE FROM known_devices WHERE last_seen_at < sqlc.arg(before);

-- Page history: a wiki page's body after each editing session.

-- name: LatestPageVersion :one
-- A page's newest version, so an edit in the same session updates it.
SELECT id, actor_id, created_at, updated_at
FROM page_versions WHERE page_id = sqlc.arg(page_id)
ORDER BY created_at DESC, id DESC LIMIT 1;

-- name: InsertPageVersion :exec
INSERT INTO page_versions (id, workspace_id, page_id, actor_id, body, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(page_id), sqlc.narg(actor_id), sqlc.arg(body), sqlc.arg(now), sqlc.arg(now));

-- name: UpdatePageVersion :exec
UPDATE page_versions SET body = sqlc.arg(body), updated_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- name: ListPageVersionSizes :many
-- A page's versions, newest first, with their sizes in bytes (read from
-- the row's header, not the body), for pruning.
SELECT id, CAST(octet_length(body) AS BIGINT) AS size
FROM page_versions WHERE page_id = sqlc.arg(page_id)
ORDER BY created_at DESC, id DESC;

-- name: DeletePageVersion :exec
DELETE FROM page_versions WHERE id = sqlc.arg(id);

-- name: ListPageVersions :many
SELECT v.id, v.actor_id, COALESCE(u.name, '') AS actor_name, v.body, v.created_at, v.updated_at
FROM page_versions v LEFT JOIN users u ON u.id = v.actor_id
WHERE v.workspace_id = sqlc.arg(workspace_id) AND v.page_id = sqlc.arg(page_id)
ORDER BY v.created_at DESC, v.id DESC
LIMIT CAST(sqlc.arg(max_rows) AS BIGINT);

-- API tokens

-- name: InsertApiToken :exec
INSERT INTO api_tokens (id, user_id, name, token_hash, can_write, workspace_id, created_at, expires_at, oauth_client_id)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(token_hash), sqlc.arg(can_write),
        sqlc.narg(workspace_id), sqlc.arg(now), sqlc.narg(expires_at), sqlc.narg(oauth_client_id));

-- name: ListApiTokens :many
-- Someone's tokens not revoked, newest first; those expired stay until revoked.
SELECT * FROM api_tokens WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL
ORDER BY created_at DESC, id DESC;

-- name: CountApiTokens :one
SELECT COUNT(*) FROM api_tokens WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: GetApiTokenByHash :one
SELECT * FROM api_tokens WHERE token_hash = sqlc.arg(token_hash) AND revoked_at IS NULL;

-- name: RevokeApiToken :one
UPDATE api_tokens SET revoked_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND revoked_at IS NULL
RETURNING name;

-- name: RevokeUserApiTokens :exec
-- Ends every token someone made when an admin signs them out everywhere: a
-- token outlives sessions, so a stolen one would.
UPDATE api_tokens SET revoked_at = sqlc.arg(now) WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: RevokeMemberApiTokens :exec
-- Ends the tokens someone made for one workspace they leave, so they do not
-- come back to life if they join it again.
UPDATE api_tokens SET revoked_at = sqlc.arg(now)
WHERE user_id = sqlc.arg(user_id) AND workspace_id = sqlc.arg(workspace_id) AND revoked_at IS NULL;

-- name: TouchApiToken :exec
-- Records a use, at most once per interval, so a busy script does not write
-- every time.
UPDATE api_tokens SET last_used_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND (last_used_at IS NULL OR last_used_at < sqlc.arg(before));

-- name: DeleteUserApiTokens :exec
DELETE FROM api_tokens WHERE user_id = sqlc.arg(user_id);

-- name: RevokeAppApiTokens :exec
-- Ends the tokens an app was given before, when the person connects it again.
UPDATE api_tokens SET revoked_at = sqlc.arg(now)
WHERE user_id = sqlc.arg(user_id) AND oauth_client_id = sqlc.arg(client_id) AND revoked_at IS NULL;

-- name: RevokeApiTokenByID :exec
UPDATE api_tokens SET revoked_at = sqlc.arg(now) WHERE id = sqlc.arg(id) AND revoked_at IS NULL;

-- OAuth (apps that connect to the MCP endpoint)

-- name: InsertOAuthClient :exec
INSERT INTO oauth_clients (id, name, redirect_uris, created_at)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(redirect_uris), sqlc.arg(now));

-- name: GetOAuthClient :one
SELECT * FROM oauth_clients WHERE id = sqlc.arg(id);

-- name: CountOAuthClients :one
SELECT COUNT(*) FROM oauth_clients;

-- name: DeleteIdleOAuthClients :exec
-- Apps that registered and were never said yes to.
DELETE FROM oauth_clients WHERE used_at IS NULL AND created_at < sqlc.arg(before);

-- name: DeleteUnusedOAuthClients :exec
-- Apps said yes to long ago that hold no token that works any more: kept,
-- one person could fill the registry by saying yes to their own apps.
DELETE FROM oauth_clients WHERE used_at IS NOT NULL AND used_at < sqlc.arg(before)
    AND NOT EXISTS (SELECT 1 FROM api_tokens t WHERE t.oauth_client_id = oauth_clients.id AND t.revoked_at IS NULL
        AND (t.expires_at IS NULL OR t.expires_at > sqlc.arg(now)));

-- name: TouchOAuthClient :exec
UPDATE oauth_clients SET used_at = sqlc.arg(now) WHERE id = sqlc.arg(id);

-- name: ListOAuthClients :many
-- Every app registered, oldest first, with how many of its tokens still work.
SELECT c.id, c.name, c.created_at, c.used_at, COUNT(t.id) AS live_tokens
FROM oauth_clients c
LEFT JOIN api_tokens t ON t.oauth_client_id = c.id AND t.revoked_at IS NULL
    AND (t.expires_at IS NULL OR t.expires_at > sqlc.arg(now))
GROUP BY c.id, c.name, c.created_at, c.used_at
ORDER BY c.created_at, c.id;

-- name: ListOAuthClientTokens :many
-- An app's tokens that still work.
SELECT id, user_id, name FROM api_tokens
WHERE oauth_client_id = sqlc.arg(client_id) AND revoked_at IS NULL
    AND (expires_at IS NULL OR expires_at > sqlc.arg(now))
ORDER BY created_at, id;

-- name: DeleteOAuthClient :execrows
-- Its codes and tokens go with it.
DELETE FROM oauth_clients WHERE id = sqlc.arg(id);

-- name: InsertOAuthCode :exec
INSERT INTO oauth_codes (code_hash, client_id, user_id, workspace_id, can_write, expires_in_days, redirect_uri, challenge, created_at, expires_at)
VALUES (sqlc.arg(code_hash), sqlc.arg(client_id), sqlc.arg(user_id), sqlc.arg(workspace_id), sqlc.arg(can_write),
        sqlc.arg(expires_in_days), sqlc.arg(redirect_uri), sqlc.arg(challenge), sqlc.arg(now), sqlc.arg(expires_at));

-- name: GetOAuthCode :one
SELECT * FROM oauth_codes WHERE code_hash = sqlc.arg(code_hash);

-- name: UseOAuthCode :execrows
-- Marks a code used, once: no rows means it was used before.
UPDATE oauth_codes SET used_at = sqlc.arg(now) WHERE code_hash = sqlc.arg(code_hash) AND used_at IS NULL;

-- name: SetOAuthCodeToken :exec
UPDATE oauth_codes SET token_id = sqlc.arg(token_id) WHERE code_hash = sqlc.arg(code_hash);

-- name: DeleteOldOAuthCodes :exec
DELETE FROM oauth_codes WHERE expires_at < sqlc.arg(before);

-- name: DeleteUserOAuthCodes :exec
DELETE FROM oauth_codes WHERE user_id = sqlc.arg(user_id);

-- Webhooks

-- name: InsertWebhook :exec
INSERT INTO webhooks (id, workspace_id, created_by, url, secret, events, project_id, created_at)
VALUES (sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(created_by), sqlc.arg(url), sqlc.arg(secret),
        sqlc.arg(events), sqlc.narg(project_id), sqlc.arg(now));

-- name: ListWebhooks :many
SELECT * FROM webhooks WHERE workspace_id = sqlc.arg(workspace_id) ORDER BY created_at, id;

-- name: CountWebhooks :one
SELECT COUNT(*) FROM webhooks WHERE workspace_id = sqlc.arg(workspace_id);

-- name: ListLiveWebhooks :many
-- What hears about a change: read on every change to something a webhook can
-- hear about, inside its transaction.
SELECT * FROM webhooks
WHERE workspace_id = sqlc.arg(workspace_id) AND paused_at IS NULL AND disabled_at IS NULL;

-- name: GetWebhook :one
SELECT * FROM webhooks WHERE id = sqlc.arg(id);

-- name: UpdateWebhook :exec
UPDATE webhooks SET created_by = sqlc.arg(created_by), url = sqlc.arg(url), events = sqlc.arg(events), project_id = sqlc.narg(project_id),
       paused_at = sqlc.narg(paused_at), disabled_at = sqlc.narg(disabled_at), failing_since = sqlc.narg(failing_since)
WHERE id = sqlc.arg(id);

-- name: SetWebhookSecret :exec
UPDATE webhooks SET secret = sqlc.arg(secret) WHERE id = sqlc.arg(id);

-- name: SetWebhookFailing :exec
UPDATE webhooks SET failing_since = sqlc.narg(failing_since), disabled_at = sqlc.narg(disabled_at)
WHERE id = sqlc.arg(id);

-- name: DeleteWebhook :exec
DELETE FROM webhooks WHERE id = sqlc.arg(id);

-- name: InsertWebhookDelivery :exec
INSERT INTO webhook_deliveries (id, webhook_id, event, summary, body, status, attempts, response_body, created_at, next_at)
VALUES (sqlc.arg(id), sqlc.arg(webhook_id), sqlc.arg(event), sqlc.arg(summary), sqlc.arg(body), 'pending', 0, '',
        sqlc.arg(now), sqlc.arg(now));

-- name: GetWebhookDelivery :one
SELECT * FROM webhook_deliveries WHERE id = sqlc.arg(id);

-- name: ListWebhookDeliveries :many
SELECT * FROM webhook_deliveries WHERE webhook_id = sqlc.arg(webhook_id)
ORDER BY created_at DESC, id DESC LIMIT CAST(sqlc.arg(max_rows) AS BIGINT);

-- name: SetWebhookDeliveryResult :exec
UPDATE webhook_deliveries SET status = sqlc.arg(status), attempts = sqlc.arg(attempts),
       response_code = sqlc.narg(response_code), response_body = sqlc.arg(response_body),
       attempted_at = sqlc.narg(attempted_at), next_at = sqlc.narg(next_at)
WHERE id = sqlc.arg(id);

-- name: DeleteOldWebhookDeliveries :exec
DELETE FROM webhook_deliveries WHERE created_at < sqlc.arg(before);

-- name: ProjectOfTask :one
-- A task's project, to tell whether a webhook for one project hears of what hangs off it.
SELECT COALESCE(project_id, '') FROM tasks WHERE id = sqlc.arg(id);

-- name: ProjectOfPage :one
-- A page's project, for the same, for what hangs off a page.
SELECT COALESCE(project_id, '') FROM pages WHERE id = sqlc.arg(id);

-- name: PauseMemberWebhooks :execrows
-- Pauses what someone made in a workspace, as they stop being able to manage it:
-- a webhook they set up must not go on sending the workspace's work to them.
UPDATE webhooks SET paused_at = sqlc.arg(now)
WHERE created_by = sqlc.arg(created_by) AND workspace_id = sqlc.arg(workspace_id) AND paused_at IS NULL;

-- name: ListWebhookWorkspacesOf :many
SELECT DISTINCT workspace_id FROM webhooks WHERE created_by = sqlc.arg(created_by) AND paused_at IS NULL;

-- name: ClaimWebhookDelivery :execrows
-- Takes a try of a delivery, so two jobs for it can never both send.
UPDATE webhook_deliveries SET attempts = attempts + 1
WHERE id = sqlc.arg(id) AND status = 'pending' AND attempts = sqlc.arg(attempts);

-- name: ListAllWebhookIDs :many
SELECT id FROM webhooks;

-- name: TrimWebhookDeliveries :exec
-- Keeps a webhook's newest deliveries, however many changes a workspace makes.
DELETE FROM webhook_deliveries WHERE webhook_deliveries.webhook_id = sqlc.arg(webhook) AND webhook_deliveries.id NOT IN (
    SELECT keep.id FROM webhook_deliveries keep WHERE keep.webhook_id = sqlc.arg(webhook)
    ORDER BY keep.created_at DESC, keep.id DESC LIMIT CAST(sqlc.arg(max_rows) AS BIGINT));

-- Email to task

-- name: InsertInboundAddress :exec
INSERT INTO inbound_addresses (id, user_id, workspace_id, token, project_id, assign_self, senders, created_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(workspace_id), sqlc.arg(token), sqlc.narg(project_id),
        sqlc.arg(assign_self), sqlc.arg(senders), sqlc.arg(now));

-- name: ListInboundAddresses :many
SELECT * FROM inbound_addresses WHERE user_id = sqlc.arg(user_id) AND workspace_id = sqlc.arg(workspace_id)
ORDER BY created_at, id;

-- name: CountInboundAddresses :one
SELECT COUNT(*) FROM inbound_addresses WHERE user_id = sqlc.arg(user_id) AND workspace_id = sqlc.arg(workspace_id);

-- name: GetInboundAddressByToken :one
SELECT * FROM inbound_addresses WHERE token = sqlc.arg(token);

-- name: GetInboundAddress :one
SELECT * FROM inbound_addresses
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND workspace_id = sqlc.arg(workspace_id);

-- name: UpdateInboundAddress :exec
UPDATE inbound_addresses SET project_id = sqlc.narg(project_id), assign_self = sqlc.arg(assign_self), senders = sqlc.arg(senders)
WHERE id = sqlc.arg(id);

-- name: SetInboundAddressToken :exec
UPDATE inbound_addresses SET token = sqlc.arg(token) WHERE id = sqlc.arg(id);

-- name: DeleteInboundAddress :exec
DELETE FROM inbound_addresses WHERE id = sqlc.arg(id);

-- name: DeleteMemberInboundAddresses :exec
-- A person's addresses in a workspace they left or became a guest of.
DELETE FROM inbound_addresses WHERE user_id = sqlc.arg(user_id) AND workspace_id = sqlc.arg(workspace_id);

-- name: DeleteUserInboundAddresses :exec
DELETE FROM inbound_addresses WHERE user_id = sqlc.arg(user_id);

-- name: DeleteMemberInboundMail :exec
DELETE FROM inbound_mail WHERE user_id = sqlc.arg(user_id) AND workspace_id = sqlc.arg(workspace_id);

-- name: DeleteUserInboundMail :exec
DELETE FROM inbound_mail WHERE user_id = sqlc.arg(user_id);

-- name: InsertInboundMail :exec
INSERT INTO inbound_mail (id, user_id, workspace_id, address_id, from_address, subject, outcome, task_id, size, received_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(workspace_id), sqlc.arg(address_id), sqlc.arg(from_address),
        sqlc.arg(subject), sqlc.arg(outcome), sqlc.narg(task_id), sqlc.arg(size), sqlc.arg(now));

-- name: ListInboundMailMade :many
-- Apart from what was refused, so that junk can never push tasks off the list.
SELECT * FROM inbound_mail
WHERE user_id = sqlc.arg(user_id) AND workspace_id = sqlc.arg(workspace_id) AND outcome = 'made'
ORDER BY received_at DESC, id DESC
LIMIT 100;

-- name: ListInboundMailRefused :many
SELECT * FROM inbound_mail
WHERE user_id = sqlc.arg(user_id) AND workspace_id = sqlc.arg(workspace_id) AND outcome <> 'made'
ORDER BY received_at DESC, id DESC
LIMIT 50;

-- name: CountInboundMailForTask :one
-- Whether a task from email was finished: this is written last.
SELECT COUNT(*) FROM inbound_mail WHERE task_id = sqlc.arg(task_id);

-- name: SumInboundBytesSince :one
-- How much an address has taken in lately, for its daily limit.
SELECT CAST(COALESCE(SUM(size), 0) AS BIGINT) FROM inbound_mail
WHERE address_id = sqlc.arg(address_id) AND received_at >= sqlc.arg(since);

-- name: CountInboundMadeSince :one
-- The tasks an address made lately, for its daily limit.
SELECT COUNT(*) FROM inbound_mail
WHERE address_id = sqlc.arg(address_id) AND outcome = 'made' AND received_at >= sqlc.arg(since);

-- name: CountInboundRefusedSince :one
-- The refusals listed lately, so a flood of junk cannot fill the list.
SELECT COUNT(*) FROM inbound_mail
WHERE address_id = sqlc.arg(address_id) AND outcome <> 'made' AND received_at >= sqlc.arg(since);

-- name: PurgeInboundMail :exec
DELETE FROM inbound_mail WHERE received_at < sqlc.arg(before);

-- name: CountLiveProject :one
-- Whether a project is in a workspace, neither deleted nor archived.
SELECT COUNT(*) FROM projects
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND deleted_at IS NULL AND archived_at IS NULL;

-- Signing in to the desktop app

-- name: InsertDesktopCode :exec
INSERT INTO desktop_codes (code_hash, user_id, challenge, created_at, expires_at)
VALUES (sqlc.arg(code_hash), sqlc.arg(user_id), sqlc.arg(challenge), sqlc.arg(now), sqlc.arg(expires_at));

-- name: TakeDesktopCode :one
-- A code is spent by trying it, right or wrong, so it can't be guessed at.
DELETE FROM desktop_codes WHERE code_hash = sqlc.arg(code_hash) AND expires_at > sqlc.arg(now)
RETURNING user_id, challenge;

-- name: PurgeDesktopCodes :exec
DELETE FROM desktop_codes WHERE expires_at <= sqlc.arg(now);

-- name: DeleteUserDesktopCodes :exec
DELETE FROM desktop_codes WHERE user_id = sqlc.arg(user_id);

-- name: ListLiveAutomations :many
-- The rules a workspace's changes can start.
SELECT id, project_id, name, made_by, rule, enabled_at FROM automations
WHERE workspace_id = sqlc.arg(workspace_id) AND enabled AND deleted_at IS NULL AND paused_why IS NULL;

-- name: ListAllLiveAutomations :many
-- Every rule that could start, for the check of due dates.
SELECT id, workspace_id, project_id, name, made_by, rule, enabled_at FROM automations
WHERE enabled AND deleted_at IS NULL AND paused_why IS NULL;

-- name: GetLiveAutomation :one
SELECT id, workspace_id, project_id, name, made_by, rule, enabled_at FROM automations
WHERE id = sqlc.arg(id) AND enabled AND deleted_at IS NULL AND paused_why IS NULL;

-- name: CountLiveAutomations :one
SELECT COUNT(*) FROM automations WHERE workspace_id = sqlc.arg(workspace_id) AND deleted_at IS NULL;

-- name: InsertAutomationRun :exec
INSERT INTO automation_runs (id, workspace_id, automation_id, task_id, task_title, outcome, detail, created_at, batch)
VALUES (sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(automation_id), sqlc.arg(task_id), sqlc.arg(task_title), sqlc.arg(outcome), sqlc.arg(detail), sqlc.arg(now), sqlc.narg(batch));

-- name: CountAutomationRunsSince :one
SELECT COUNT(*) FROM automation_runs WHERE automation_id = sqlc.arg(automation_id) AND created_at >= sqlc.arg(since);

-- name: CountAutomationChangesSince :one
-- The changes that started a rule's runs, but for one batch: each run alone,
-- and a change to many tasks (one batch) once.
SELECT COUNT(DISTINCT COALESCE(batch, id)) FROM automation_runs
WHERE automation_id = sqlc.arg(automation_id) AND created_at >= sqlc.arg(since)
  AND (batch IS NULL OR batch <> sqlc.arg(batch));

-- name: ListAutomationRuns :many
SELECT id, automation_id, task_id, task_title, outcome, detail, created_at FROM automation_runs
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY created_at DESC, id DESC
LIMIT CAST(sqlc.arg(max_rows) AS BIGINT);

-- name: ListAutomationRunsOf :many
SELECT id, automation_id, task_id, task_title, outcome, detail, created_at FROM automation_runs
WHERE workspace_id = sqlc.arg(workspace_id) AND automation_id = sqlc.arg(automation_id)
ORDER BY created_at DESC, id DESC
LIMIT CAST(sqlc.arg(max_rows) AS BIGINT);

-- name: PurgeAutomationRuns :exec
DELETE FROM automation_runs WHERE created_at < sqlc.arg(before);

-- name: AutomationWasFired :one
SELECT COUNT(*) FROM automation_fired
WHERE automation_id = sqlc.arg(automation_id) AND task_id = sqlc.arg(task_id) AND due_date = sqlc.arg(due_date);

-- name: MarkAutomationFired :execrows
INSERT INTO automation_fired (automation_id, task_id, due_date, created_at)
VALUES (sqlc.arg(automation_id), sqlc.arg(task_id), sqlc.arg(due_date), sqlc.arg(now))
ON CONFLICT DO NOTHING;

-- name: PurgeAutomationFired :exec
DELETE FROM automation_fired WHERE created_at < sqlc.arg(before);

-- name: AutomationTaskProject :one
SELECT project_id FROM tasks WHERE id = sqlc.arg(id);

-- name: AutomationStatusCategory :one
SELECT category FROM statuses WHERE id = sqlc.arg(id);

-- name: GetPairID :one
-- What a pair's row is called now, by what it was called before its ID was
-- a hash of the pair (migration 38).
SELECT new_id FROM pair_ids WHERE entity = sqlc.arg(entity) AND old_id = sqlc.arg(old_id);
