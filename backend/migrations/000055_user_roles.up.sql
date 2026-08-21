-- Until now every logged-in user saw everything: users carried no role, the JWT
-- carried only a subject, and all ~61 protected operations declared an empty
-- scope list. That was fine while the only account was the owner's; it stops
-- being fine the moment an advocate gets a login, because the same session that
-- shows their own cases also returns the P&L, every contractor's pay, and the
-- password vault in plain text.
--
-- The default is 'admin' deliberately: this migration must not quietly restrict
-- the account that is already in use. New advocate logins are created
-- explicitly (cmd/createuser --role advocate).
ALTER TABLE users ADD COLUMN IF NOT EXISTS role text NOT NULL DEFAULT 'admin';
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'advocate'));

-- The join that did not exist. An advocate's identity was a full_name and a
-- Telegram chat id, with nothing tying it to a login, so there was nothing to
-- scope data on. NULL for admins, who are not on the roster.
ALTER TABLE users ADD COLUMN IF NOT EXISTS advocate_id uuid REFERENCES advocates(id);

-- An advocate login without a roster link would see nothing and mean nothing.
ALTER TABLE users ADD CONSTRAINT users_advocate_link_check
    CHECK (role <> 'advocate' OR advocate_id IS NOT NULL);
