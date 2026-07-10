DROP TABLE IF EXISTS contact_tombstones;
ALTER TABLE contacts DROP COLUMN change_seq;
ALTER TABLE address_books DROP COLUMN change_seq;
