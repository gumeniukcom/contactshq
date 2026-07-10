-- A change journal for the CardDAV collection, so clients can ask "what changed since I
-- last looked" instead of downloading every ETag on every poll.
--
-- RFC 6578 sync-collection has to name deleted resources explicitly, which no amount of
-- looking at the contacts table can reconstruct. Deletions leave a tombstone behind.
--
-- change_seq is a per-address-book counter, bumped inside the same transaction as the
-- write it describes. A collection's ctag is its current change_seq, and a sync token is
-- the change_seq the client last saw.

ALTER TABLE address_books ADD COLUMN change_seq BIGINT NOT NULL DEFAULT 0;
ALTER TABLE contacts ADD COLUMN change_seq BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS contact_tombstones (
    id              TEXT PRIMARY KEY,
    address_book_id TEXT NOT NULL REFERENCES address_books(id) ON DELETE CASCADE,
    uid             TEXT NOT NULL,
    change_seq      BIGINT NOT NULL,
    deleted_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_contact_tombstones_book_seq ON contact_tombstones(address_book_id, change_seq);
CREATE INDEX IF NOT EXISTS idx_contacts_book_seq ON contacts(address_book_id, change_seq);

-- Existing rows start at sequence 1 rather than 0, so a client holding token "1" is told
-- it is up to date, while a client with no token gets the whole collection.
UPDATE address_books SET change_seq = 1;
UPDATE contacts SET change_seq = 1;
