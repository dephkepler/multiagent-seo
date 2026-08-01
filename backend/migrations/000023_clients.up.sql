CREATE TABLE IF NOT EXISTS clients (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    phone         text        NOT NULL,
    name          text        NOT NULL DEFAULT '',
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_clients_phone ON clients (phone);

ALTER TABLE leads ADD COLUMN IF NOT EXISTS client_id uuid REFERENCES clients(id);
