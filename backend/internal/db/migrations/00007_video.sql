-- +goose Up
-- Length of a video original in milliseconds; NULL for still images.
ALTER TABLE photos ADD COLUMN duration_ms INTEGER;

-- +goose Down
ALTER TABLE photos DROP COLUMN duration_ms;
