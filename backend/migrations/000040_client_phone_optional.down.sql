DROP INDEX IF EXISTS uq_clients_phone;
CREATE UNIQUE INDEX IF NOT EXISTS uq_clients_phone ON clients (phone);

ALTER TABLE clients ALTER COLUMN phone SET NOT NULL;
