-- Additional client-card fields. Gender/email/client-type are plain
-- columns, same as name/phone. Address, birthdate, and tax id (РНОКПП) are
-- genuinely sensitive PII a law firm's own documents need but a data leak
-- shouldn't hand out in the clear — encrypted at rest with pgcrypto
-- (pgp_sym_encrypt/pgp_sym_decrypt), the same mechanism already used for
-- WordPress site credentials (see wordpress_site_repository.go), keyed by
-- a separate CF_CLIENTS_ENCRYPTION_KEY so rotating/compromising one
-- doesn't touch the other.
ALTER TABLE clients ADD COLUMN IF NOT EXISTS gender text NOT NULL DEFAULT '';
ALTER TABLE clients ADD CONSTRAINT clients_gender_check CHECK (gender IN ('', 'male', 'female'));

ALTER TABLE clients ADD COLUMN IF NOT EXISTS email text NOT NULL DEFAULT '';

ALTER TABLE clients ADD COLUMN IF NOT EXISTS client_type text NOT NULL DEFAULT 'individual';
ALTER TABLE clients ADD CONSTRAINT clients_client_type_check CHECK (client_type IN ('individual', 'legal_entity'));
ALTER TABLE clients ADD COLUMN IF NOT EXISTS company_name text NOT NULL DEFAULT '';
ALTER TABLE clients ADD COLUMN IF NOT EXISTS company_code text NOT NULL DEFAULT '';

ALTER TABLE clients ADD COLUMN IF NOT EXISTS address_enc bytea;
ALTER TABLE clients ADD COLUMN IF NOT EXISTS birthdate_enc bytea;
ALTER TABLE clients ADD COLUMN IF NOT EXISTS tax_id_enc bytea;
