-- Closes the tag vocabulary: client_tags.tag used to accept any free text
-- staff typed, which drifts (VIP / vip / важный клиент as three unrelated
-- tags). This table is the curated, stable list; the FK below is what
-- actually enforces "pick from the list", and ON UPDATE/DELETE CASCADE
-- means renaming or removing a tag definition here fixes every client_tags
-- row that used it in one statement, not a manual sweep.
CREATE TABLE IF NOT EXISTS client_tag_defs (
    label      text PRIMARY KEY,
    created_by text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Backfill from whatever staff already typed under the free-text version of
-- this feature (shipped moments before this migration) so the FK below
-- doesn't orphan real data.
INSERT INTO client_tag_defs (label)
SELECT DISTINCT tag FROM client_tags
ON CONFLICT (label) DO NOTHING;

-- Starter vocabulary — rename/remove freely via the CRM, this is just a
-- reasonable default so the list isn't empty on day one.
INSERT INTO client_tag_defs (label) VALUES
    ('VIP'),
    ('Проблемний клієнт'),
    ('Юрособа-партнер'),
    ('За рекомендацією'),
    ('Потребує особливої уваги')
ON CONFLICT (label) DO NOTHING;

ALTER TABLE client_tags
    ADD CONSTRAINT client_tags_tag_fkey
    FOREIGN KEY (tag) REFERENCES client_tag_defs (label)
    ON UPDATE CASCADE ON DELETE CASCADE;
