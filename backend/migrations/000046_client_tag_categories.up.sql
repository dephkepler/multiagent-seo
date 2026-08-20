-- Tags without any grouping read as one flat, unorganized list once there
-- are more than a handful — this adds a category to each vocabulary entry
-- so the picker/management UI can show two levels (category → tag) instead
-- of one long list.
ALTER TABLE client_tag_defs ADD COLUMN IF NOT EXISTS category text NOT NULL DEFAULT 'Інше';

UPDATE client_tag_defs SET category = 'Статус'
    WHERE label IN ('VIP', 'Проблемний клієнт', 'Потребує особливої уваги');
UPDATE client_tag_defs SET category = 'Тип клієнта'
    WHERE label = 'Юрособа-партнер';
UPDATE client_tag_defs SET category = 'Джерело'
    WHERE label = 'За рекомендацією';
