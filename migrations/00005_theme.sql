-- Theme: each User's choice of light, dark, or following their device.

-- +goose Up
ALTER TABLE users ADD COLUMN theme TEXT NOT NULL DEFAULT 'system'
    CHECK (theme IN ('system', 'light', 'dark'));

-- +goose Down
ALTER TABLE users DROP COLUMN theme;
