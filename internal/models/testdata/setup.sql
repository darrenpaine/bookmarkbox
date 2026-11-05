-- Test data, loaded after the migrations. Alice's password is "pa$$word".
INSERT INTO users (name, email, hashed_password, created)
VALUES (
    'Alice Jones',
    'alice@example.com',
    '$2a$12$NuTjWXm3KKntReFwyBVHyuf/to.HEwTy.eS206TNfkGfr6HzGJSWG',
    '2026-01-01 10:00:00'
);

INSERT INTO users (name, email, hashed_password, created)
VALUES (
    'Bob Smith',
    'bob@example.com',
    '$2a$12$NuTjWXm3KKntReFwyBVHyuf/to.HEwTy.eS206TNfkGfr6HzGJSWG',
    '2026-01-01 11:00:00'
);

INSERT INTO bookmarks (user_id, url, normalised_url, title, description, created_at)
VALUES (1, 'https://go.dev/doc/effective_go', 'https://go.dev/doc/effective_go', 'Effective Go', 'Idiomatic Go', '2026-01-02 09:00:00');

INSERT INTO tags (user_id, name) VALUES (1, 'go');
INSERT INTO bookmark_tags (bookmark_id, tag_id) VALUES (1, 1);
