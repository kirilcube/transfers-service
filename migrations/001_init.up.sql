CREATE TYPE idempotency_status AS ENUM (
    'PROCESSING', 'COMPLETED', 'FAILED'
);

CREATE TYPE transfer_status AS ENUM (
    'CREATED', 'ANTIFRAUD_FAILED', 'VOP_NO_MATCH', 'ROUTING_FAILED', 'INITIALIZED',
    'SENT_FOR_CLEARING', 'SETTLED'
);

CREATE TABLE sct_transfers
(
    id               UUID PRIMARY KEY         DEFAULT gen_random_uuid(),
    status           transfer_status NOT NULL DEFAULT 'CREATED',

    partner_bank     UUID,
    vop_response     JSONB,
    vop_override     BOOL                     DEFAULT FALSE,

    merchant_id      UUID            NOT NULL,
    iban             VARCHAR(34)     NOT NULL,
    payee            VARCHAR(70)     NOT NULL,

    principal_amount BIGINT          NOT NULL,
    total_debit      BIGINT          NOT NULL,
    fee_amount       BIGINT          NOT NULL,
    currency         CHAR(3)         NOT NULL,
    reference_text   VARCHAR(140),

    version          INT             NOT NULL DEFAULT 1,
    locked_at        TIMESTAMPTZ,

    metadata         JSONB                    DEFAULT '{}',
    created_at       TIMESTAMPTZ     NOT NULL DEFAULT clock_timestamp(),
    updated_at       TIMESTAMPTZ     NOT NULL DEFAULT clock_timestamp(),

    CONSTRAINT chk_positive_principal_amount CHECK (principal_amount > 0),
    CONSTRAINT chk_positive_fee_amount CHECK (fee_amount >= 0),
    CONSTRAINT chk_amounts_balance CHECK (total_debit = principal_amount + fee_amount)
);
CREATE INDEX idx_sct_transfers_sweeper ON sct_transfers (status, locked_at) WHERE status IN ('CREATED', 'INITIALIZED');

CREATE TABLE idempotency_records
(
    idempotency_key VARCHAR(128) NOT NULL,
    merchant_id     UUID         NOT NULL,
    transfer_id     UUID REFERENCES sct_transfers,

    response_body   JSONB                 DEFAULT '{}',
    response_code   INT                   DEFAULT 409,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_idempotency PRIMARY KEY (idempotency_key, merchant_id)
);

CREATE TABLE outbox
(
    id            UUID PRIMARY KEY      DEFAULT gen_random_uuid(),
    aggregatetype VARCHAR(255) NOT NULL,
    aggregateid   VARCHAR(255) NOT NULL,
    type          VARCHAR(255) NOT NULL,
    headers       JSONB                 DEFAULT '{}',
    payload       JSONB        NOT NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT clock_timestamp()
);

CREATE OR REPLACE FUNCTION delete_outbox_row() RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM outbox WHERE id = NEW.id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER outbox_delete_trigger
    AFTER INSERT ON outbox
    FOR EACH ROW EXECUTE FUNCTION delete_outbox_row();

CREATE TABLE mock_ledger_entries
(
    idempotency_key  VARCHAR(128) PRIMARY KEY,
    transfer_id      UUID        NOT NULL,
    action           VARCHAR(50) NOT NULL,
    principal_amount BIGINT      NOT NULL,
    fee_amount       BIGINT      NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
