-- Manual call/contact log per client, typed in by staff from the client
-- card in the admin UI. The system has no actual phone integration, so
-- this is deliberately just a flat append-only log of free text, not
-- structured "call" data nobody is capturing.
CREATE TABLE IF NOT EXISTS client_notes (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id  uuid        NOT NULL REFERENCES clients(id),
    text       text        NOT NULL,
    created_by text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_client_notes_client_id ON client_notes (client_id);
