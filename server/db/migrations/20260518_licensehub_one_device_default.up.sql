-- New plans bind one device per license by default. Existing rows are not
-- rewritten, so operators can opt them in without surprising active users.
ALTER TABLE plans ALTER COLUMN max_activations SET DEFAULT 1;
