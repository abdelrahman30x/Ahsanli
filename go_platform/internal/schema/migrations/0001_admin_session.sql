CREATE TABLE go_admin_session (
    token_hash bytea PRIMARY KEY,
    user_id integer NOT NULL REFERENCES auth_user(id) ON DELETE CASCADE,
    csrf_token text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);
CREATE INDEX go_admin_session_expiry_idx ON go_admin_session(expires_at);
