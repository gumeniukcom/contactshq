-- The jobs table has never been read or written. It was created for a durable queue that
-- was never built: the worker keeps jobs in a bounded in-memory channel. Leaving the table
-- in the schema implies a persistence guarantee the system does not offer.
DROP TABLE IF EXISTS jobs;
