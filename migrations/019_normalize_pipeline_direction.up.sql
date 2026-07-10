-- Normalize pipeline steps so the internal address book is always the destination.
--
-- The engine executes source -> dest for "pull" and dest -> source for "push", which
-- means a step saved as source=internal, dest=carddav (the shape the pipeline form
-- created by default) ran in the opposite direction to what its labels said. Rather than
-- keep two readings of the same columns, every step is rewritten to the one arrangement
-- the rest of the system already assumes: source is the external provider, dest is
-- 'internal'. Direction then says plainly which way contacts move.
--
--   pull | push | bidirectional  ->  import | export | two_way
--
-- Rows that were stored inverted are swapped, and their conflict_mode is swapped with
-- them: "source wins" meant "internal wins" there, which is "dest wins" once the sides
-- are exchanged. Behaviour is preserved exactly; only the labels become truthful.

UPDATE pipeline_steps
SET source_type   = dest_type,
    source_config = dest_config,
    dest_type     = 'internal',
    dest_config   = '{}',
    direction     = CASE direction
                        WHEN 'pull'          THEN 'export'
                        WHEN 'push'          THEN 'import'
                        WHEN 'bidirectional' THEN 'two_way'
                        ELSE 'import'
                    END,
    conflict_mode = CASE conflict_mode
                        WHEN 'source_wins' THEN 'dest_wins'
                        WHEN 'dest_wins'   THEN 'source_wins'
                        ELSE conflict_mode
                    END
WHERE source_type = 'internal' AND dest_type <> 'internal';

-- Steps that were already the right way round only need the new direction vocabulary.
-- The rows rewritten above already carry it, so the CASE leaves them alone.
UPDATE pipeline_steps
SET direction = CASE direction
                    WHEN 'pull'          THEN 'import'
                    WHEN 'push'          THEN 'export'
                    WHEN 'bidirectional' THEN 'two_way'
                    ELSE direction
                END
WHERE dest_type = 'internal' AND source_type <> 'internal';

-- Sync state is not derived data: dropping it would make the next run treat every remote
-- contact as new and duplicate the whole address book. Swap the two sides in place.
-- provider_type is 'source->dest', so an inverted key starts with the 10 characters
-- 'internal->'.
UPDATE sync_states
SET provider_type = substr(provider_type, 11) || '->internal',
    remote_id     = local_id,
    local_id      = remote_id,
    remote_etag   = local_etag,
    local_etag    = remote_etag
WHERE provider_type LIKE 'internal->%';

-- Pending conflicts on inverted pipelines are derived and were unresolvable anyway:
-- resolution looks up local_contact_id as an internal contact UID, and on those rows it
-- held the remote provider's id. Drop them; the next run re-detects them correctly.
DELETE FROM sync_conflicts WHERE provider_type LIKE 'internal->%';
