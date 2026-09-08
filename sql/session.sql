USE snippetbox;

CREATE TABLE
    sessions (
        token CHAR(43) PRIMARY KEY,
        data BLOB NOT NULL,
        expiry TIMESTAMP(6) NOT NULL
    );

CREATE INDEX sessions_expiry_idx ON sessions (expiry);

-- Create a user and password to use in the -dsn parameter