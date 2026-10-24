-- +goose Up
-- Free-text index over folder names, album names/descriptions and photo
-- titles/captions/keywords, kept in sync by triggers. Rows are keyed by the
-- source id (not rowid, which VACUUM may renumber on these TEXT-keyed tables).
-- remove_diacritics: "malmo" finds "Malmö".
CREATE VIRTUAL TABLE search_fts USING fts5(
    kind UNINDEXED, id UNINDEXED, title, body, keywords,
    tokenize = 'unicode61 remove_diacritics 2'
);

INSERT INTO search_fts (kind, id, title, body, keywords)
    SELECT 'folder', id, name, '', '' FROM folders;
INSERT INTO search_fts (kind, id, title, body, keywords)
    SELECT 'album', id, name, description, '' FROM albums;
INSERT INTO search_fts (kind, id, title, body, keywords)
    SELECT 'photo', id, title, caption, keywords FROM photos;

-- ponytail: the DELETEs scan search_fts (id is UNINDEXED); fine for tens of
-- thousands of rows. Key the index by rowid via a mapping table if it is not.
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
-- +goose StatementBegin
CREATE TRIGGER albums_search_ai AFTER INSERT ON albums BEGIN
    INSERT INTO search_fts (kind, id, title, body, keywords) VALUES ('album', new.id, new.name, new.description, '');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER albums_search_au AFTER UPDATE OF name, description ON albums BEGIN
    DELETE FROM search_fts WHERE kind = 'album' AND id = old.id;
    INSERT INTO search_fts (kind, id, title, body, keywords) VALUES ('album', new.id, new.name, new.description, '');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER albums_search_ad AFTER DELETE ON albums BEGIN
    DELETE FROM search_fts WHERE kind = 'album' AND id = old.id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER photos_search_ai AFTER INSERT ON photos BEGIN
    INSERT INTO search_fts (kind, id, title, body, keywords) VALUES ('photo', new.id, new.title, new.caption, new.keywords);
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER photos_search_au AFTER UPDATE OF title, caption, keywords ON photos BEGIN
    DELETE FROM search_fts WHERE kind = 'photo' AND id = old.id;
    INSERT INTO search_fts (kind, id, title, body, keywords) VALUES ('photo', new.id, new.title, new.caption, new.keywords);
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER photos_search_ad AFTER DELETE ON photos BEGIN
    DELETE FROM search_fts WHERE kind = 'photo' AND id = old.id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER photos_search_ad;
DROP TRIGGER photos_search_au;
DROP TRIGGER photos_search_ai;
DROP TRIGGER albums_search_ad;
DROP TRIGGER albums_search_au;
DROP TRIGGER albums_search_ai;
DROP TRIGGER folders_search_ad;
DROP TRIGGER folders_search_au;
DROP TRIGGER folders_search_ai;
DROP TABLE search_fts;
