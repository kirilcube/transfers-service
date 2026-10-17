CREATE TYPE idempotency_status AS ENUM (
    'PROCESSING', 'COMPLETED', 'FAILED'
);

CREATE TYPE transfer_status AS ENUM (
    'CREATED', 'ANTIFRAUD_FAILED', 'VOP_NO_MATCH', 'ROUTING_FAILED', 'INITIALIZED',
    'FUNDS_HOLDED', 'CLEARED', 'SETTLED'
);

CREATE TABLE partner_banks
(
    id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(64) NOT NULL,

    CONSTRAINT code_unique UNIQUE (code)
);

CREATE TABLE sct_transfers
(
    id               UUID PRIMARY KEY         DEFAULT gen_random_uuid(),
    status           transfer_status NOT NULL DEFAULT 'CREATED',

    partner_bank     UUID REFERENCES partner_banks,
    vop_response     JSONB,

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
    CONSTRAINT chk_positive_fee_amount CHECK (fee_amount >= 0)
    CONSTRAINT chk_amounts_balance CHECK (total_debit = principal_amount + fee_amount)
);

CREATE TABLE idempotency_records
(
    idempotency_key VARCHAR(128) NOT NULL,
    merchant_id     UUID         NOT NULL,
    transfer_id     UUID REFERENCES sct_transfers,
    status                       NOT NULL idempotency_status DEFAULT 'PROCESSING',

    response_body   JSONB                 DEFAULT '{}',
    response_code   INT                   DEFAULT 409,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_idempotency PRIMARY KEY (idempotency_key, merchant_id)
);

CREATE TABLE outbox
(
    id         UUID PRIMARY KEY      DEFAULT gen_random_uuid(),
    topic      VARCHAR(64)  NOT NULL,
    key        VARCHAR(128) NOT NULL,

    headers    JSONB                 DEFAULT '{}',
    payload    JSONB        NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX idx_sct_transfers_sweeper ON sct_transfers (status, locked_at) WHERE status IN ('CREATED', 'INITIALIZED', 'FUNDS_HOLDED', 'CLEARED');