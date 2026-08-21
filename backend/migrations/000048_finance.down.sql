ALTER TABLE advocates DROP CONSTRAINT IF EXISTS advocates_commission_percent_check;
ALTER TABLE advocates DROP COLUMN IF EXISTS commission_percent;
DROP TABLE IF EXISTS other_income;
DROP TABLE IF EXISTS expenses;
DROP TABLE IF EXISTS expense_rules;
DROP TABLE IF EXISTS expense_categories;
