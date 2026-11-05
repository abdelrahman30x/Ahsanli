ALTER TABLE core_dealcandidate ADD COLUMN IF NOT EXISTS reviewer_id integer REFERENCES auth_user(id);
ALTER TABLE core_dealcandidate ADD COLUMN IF NOT EXISTS reviewed_at timestamptz;
ALTER TABLE core_dealcandidate ADD COLUMN IF NOT EXISTS featured boolean NOT NULL DEFAULT false;
ALTER TABLE core_dealcandidate ADD COLUMN IF NOT EXISTS editorial_note_ar text NOT NULL DEFAULT '';
ALTER TABLE core_dealcandidate ADD COLUMN IF NOT EXISTS editorial_note_en text NOT NULL DEFAULT '';

UPDATE core_dealcandidate SET state='review' WHERE state='captured';
UPDATE core_dealcandidate d SET state='review'
WHERE d.state='queued' AND NOT EXISTS (
    SELECT 1 FROM core_publication p WHERE p.candidate_id=d.id AND p.status='sent'
);

CREATE INDEX IF NOT EXISTS core_dealcandidate_editorial_queue_idx ON core_dealcandidate(state,created_at,id);
CREATE INDEX IF NOT EXISTS core_dealcandidate_public_deals_idx ON core_dealcandidate(featured,reviewed_at DESC,id DESC) WHERE state IN ('approved','queued');
