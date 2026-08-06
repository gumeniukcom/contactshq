-- A record of merges, kept so a merge is explainable and recoverable after the fact.
--
-- Deliberately WITHOUT a foreign key to contacts. potential_duplicates references contacts
-- ON DELETE CASCADE (migration 006), which is why a merged pair cannot simply be marked
-- "merged": deleting the loser deletes the pair row. History must not die the same way, so it
-- stores identifiers as plain text and keeps its own copy of what the loser looked like.
--
-- loser_vcard carries a snapshot with the PHOTO value stripped: it is enough to recreate the
-- contact by hand, without adding hundreds of kilobytes per row.
CREATE TABLE IF NOT EXISTS merge_log (
    id                 TEXT PRIMARY KEY,
    user_id            TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    winner_id          TEXT NOT NULL DEFAULT '',
    winner_display_name TEXT NOT NULL DEFAULT '',
    loser_uid          TEXT NOT NULL DEFAULT '',
    loser_display_name TEXT NOT NULL DEFAULT '',
    loser_vcard        TEXT NOT NULL DEFAULT '',
    resolution         TEXT NOT NULL DEFAULT '{}',
    merged_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_merge_log_user_merged_at ON merge_log(user_id, merged_at DESC);
