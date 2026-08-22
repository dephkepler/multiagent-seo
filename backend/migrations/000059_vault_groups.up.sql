-- Groups passwords into named buckets (staff wanted group -> cards
-- navigation, e.g. one group "Соцсети" holding all social-media logins,
-- instead of everything in one flat list).
CREATE TABLE vault_groups (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO vault_groups (name) VALUES ('Общие');

ALTER TABLE vault_entries ADD COLUMN group_id uuid REFERENCES vault_groups(id);
UPDATE vault_entries SET group_id = (SELECT id FROM vault_groups WHERE name = 'Общие');
ALTER TABLE vault_entries ALTER COLUMN group_id SET NOT NULL;
