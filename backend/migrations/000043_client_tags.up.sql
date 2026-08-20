-- Staff-added free-text tags, layered on top of the four auto-computed
-- Tag* values from clientsegments.Derive (debtor/no_show_risk/high_value/
-- dormant) — those stay fully automatic; this table is the one place staff
-- can hang their own label on a client (e.g. "VIP", "юрлицо-партнёр")
-- without it being silently recomputed away on the next page load.
CREATE TABLE IF NOT EXISTS client_tags (
    client_id  uuid        NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    tag        text        NOT NULL,
    created_by text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (client_id, tag)
);
