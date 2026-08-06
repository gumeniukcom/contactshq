-- One row per unordered pair.
--
-- The detector now normalises the pair order (smaller contact id first) and inserts with
-- ON CONFLICT DO NOTHING instead of a SELECT-then-INSERT per candidate. The unique index is
-- what makes that safe when the scheduled scan and a manual "scan now" overlap.
--
-- No data fix-up precedes it: the previous detector walked contacts in list order and always
-- stored (i, j) with i < j by position, so inverted duplicates of the same pair are not
-- expected. Should an installation have some, this migration fails loudly rather than
-- silently discarding rows — the file runs in one transaction, so nothing is half-applied.
CREATE UNIQUE INDEX IF NOT EXISTS idx_potential_dup_pair
    ON potential_duplicates(user_id, contact_a_id, contact_b_id);

-- GetByContacts and DeleteByContact had no index behind either predicate.
CREATE INDEX IF NOT EXISTS idx_potential_dup_a ON potential_duplicates(contact_a_id);
CREATE INDEX IF NOT EXISTS idx_potential_dup_b ON potential_duplicates(contact_b_id);
