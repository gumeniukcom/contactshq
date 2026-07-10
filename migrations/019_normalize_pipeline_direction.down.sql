-- Restore the pull/push vocabulary. Steps are left in the normalized arrangement
-- (source = external provider, dest = internal), which the old engine reads correctly:
-- there, pull means source -> dest, i.e. remote -> internal.
--
-- Conflicts deleted by the up migration are not restored; they are re-detected on the
-- next run.

UPDATE pipeline_steps
SET direction = CASE direction
                    WHEN 'import'  THEN 'pull'
                    WHEN 'export'  THEN 'push'
                    WHEN 'two_way' THEN 'bidirectional'
                    ELSE direction
                END
WHERE dest_type = 'internal';
