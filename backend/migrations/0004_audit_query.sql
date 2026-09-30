-- Target-user filtering uses the existing canonical detail, without rewriting history.
-- Existing indexes already cover occurred_at, actor_user_id and event.
CREATE INDEX audit_events_target_user_idx ON audit_events (detail, occurred_at DESC, id DESC)
    WHERE detail LIKE 'target_user_id=%';
