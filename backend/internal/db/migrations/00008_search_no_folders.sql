-- +goose Up
-- Folder names are no longer searched.
DROP TRIGGER folders_search_ai;
DROP TRIGGER folders_search_au;
DROP TRIGGER folders_search_ad;
DELETE FROM search_fts WHERE kind = 'folder';

-- +goose Down
INSERT INTO search_fts (kind, id, title, body, keywords)
    SELECT 'folder', id, name, '', '' FROM folders;
-- +goose StatementBegin
CREATE TRIGGER folders_search_ai AFTER INSERT ON folders BEGIN
    INSERT INTO search_fts (kind, id, title, body, keywords) VALUES ('folder', new.id, new.name, '', '');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER folders_search_au AFTER UPDATE OF name ON folders BEGIN
    DELETE FROM search_fts WHERE kind = 'folder' AND id = old.id;
    INSERT INTO search_fts (kind, id, title, body, keywords) VALUES ('folder', new.id, new.name, '', '');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER folders_search_ad AFTER DELETE ON folders BEGIN
    DELETE FROM search_fts WHERE kind = 'folder' AND id = old.id;
END;
-- +goose StatementEnd
