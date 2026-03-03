CREATE TABLE IF NOT EXISTS contact_emails (
    id         TEXT PRIMARY KEY,
    contact_id TEXT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    value      TEXT NOT NULL,
    type       TEXT NOT NULL DEFAULT '',
    pref       INTEGER NOT NULL DEFAULT 0,
    label      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_contact_emails_contact ON contact_emails(contact_id);
CREATE INDEX IF NOT EXISTS idx_contact_emails_value   ON contact_emails(value);

CREATE TABLE IF NOT EXISTS contact_phones (
    id         TEXT PRIMARY KEY,
    contact_id TEXT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    value      TEXT NOT NULL,
    type       TEXT NOT NULL DEFAULT '',
    pref       INTEGER NOT NULL DEFAULT 0,
    label      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_contact_phones_contact ON contact_phones(contact_id);
CREATE INDEX IF NOT EXISTS idx_contact_phones_value   ON contact_phones(value);

CREATE TABLE IF NOT EXISTS contact_addresses (
    id          TEXT PRIMARY KEY,
    contact_id  TEXT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    type        TEXT NOT NULL DEFAULT '',
    pref        INTEGER NOT NULL DEFAULT 0,
    label       TEXT NOT NULL DEFAULT '',
    po_box      TEXT NOT NULL DEFAULT '',
    extended    TEXT NOT NULL DEFAULT '',
    street      TEXT NOT NULL DEFAULT '',
    city        TEXT NOT NULL DEFAULT '',
    region      TEXT NOT NULL DEFAULT '',
    postal_code TEXT NOT NULL DEFAULT '',
    country     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_contact_addresses_contact ON contact_addresses(contact_id);
CREATE INDEX IF NOT EXISTS idx_contact_addresses_city    ON contact_addresses(city);

CREATE TABLE IF NOT EXISTS contact_urls (
    id         TEXT PRIMARY KEY,
    contact_id TEXT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    value      TEXT NOT NULL,
    type       TEXT NOT NULL DEFAULT '',
    pref       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_contact_urls_contact ON contact_urls(contact_id);

CREATE TABLE IF NOT EXISTS contact_ims (
    id         TEXT PRIMARY KEY,
    contact_id TEXT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    value      TEXT NOT NULL,
    type       TEXT NOT NULL DEFAULT '',
    pref       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_contact_ims_contact ON contact_ims(contact_id);

CREATE TABLE IF NOT EXISTS contact_categories (
    id         TEXT PRIMARY KEY,
    contact_id TEXT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    value      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_contact_categories_contact ON contact_categories(contact_id);
CREATE INDEX IF NOT EXISTS idx_contact_categories_value   ON contact_categories(value);

CREATE TABLE IF NOT EXISTS contact_dates (
    id         TEXT PRIMARY KEY,
    contact_id TEXT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,
    value      TEXT NOT NULL,
    label      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_contact_dates_contact ON contact_dates(contact_id);
