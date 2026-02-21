CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name VARCHAR(36) NOT NULL,
    last_name VARCHAR(36) NOT NULL,
    age INTEGER,
    amount NUMERIC(12,2),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS transformed_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL,
    related_fields TEXT,
    CONSTRAINT fk_account
    FOREIGN KEY (account_id)
    REFERENCES accounts(id)
    ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_transformed_account_id
    ON transformed_accounts(account_id);