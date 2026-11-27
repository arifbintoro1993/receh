-- +goose Up

-- Per-chat settings. No SQL defaults on purpose: values come from instance
-- config so nothing instance-specific is hardcoded here.
CREATE TABLE users (
    chat_id         BIGINT PRIMARY KEY,
    tz              TEXT        NOT NULL,
    cycle_start_day SMALLINT    NOT NULL CHECK (cycle_start_day BETWEEN 1 AND 31),
    report_types    TEXT[]      NOT NULL,
    report_hour     SMALLINT    NOT NULL CHECK (report_hour BETWEEN 0 AND 23),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE categories (
    id      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES users (chat_id) ON DELETE CASCADE,
    name    TEXT   NOT NULL CHECK (name <> '')
);
-- Case-insensitive so "#food" resolves to "Food".
CREATE UNIQUE INDEX categories_chat_name_idx ON categories (chat_id, lower(name));

CREATE TABLE keyword_map (
    chat_id     BIGINT NOT NULL REFERENCES users (chat_id) ON DELETE CASCADE,
    keyword     TEXT   NOT NULL,
    category_id BIGINT NOT NULL REFERENCES categories (id) ON DELETE CASCADE,
    PRIMARY KEY (chat_id, keyword)
);

CREATE TABLE expenses (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chat_id     BIGINT      NOT NULL REFERENCES users (chat_id) ON DELETE CASCADE,
    amount      BIGINT      NOT NULL CHECK (amount BETWEEN 1 AND 1000000000),
    note        TEXT        NOT NULL DEFAULT '',
    category_id BIGINT      NOT NULL REFERENCES categories (id),
    spent_at    TIMESTAMPTZ NOT NULL,
    bot_msg_id  BIGINT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
-- Every period read filters deleted_at IS NULL, so this partial index serves
-- them as a range scan: O(log n + k).
CREATE INDEX expenses_period_idx ON expenses (chat_id, spent_at) WHERE deleted_at IS NULL;
-- Reply-to-confirmation edits look up the expense by the bot's message.
CREATE INDEX expenses_bot_msg_idx ON expenses (chat_id, bot_msg_id);

-- Telegram update dedupe; rows older than 7 days are purged.
CREATE TABLE processed_updates (
    update_id    BIGINT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Exactly-once guard for scheduled reports.
CREATE TABLE report_runs (
    chat_id      BIGINT      NOT NULL REFERENCES users (chat_id) ON DELETE CASCADE,
    report_type  TEXT        NOT NULL CHECK (report_type IN ('weekly', 'monthly', 'cycle')),
    period_start TIMESTAMPTZ NOT NULL,
    sent_at      TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (chat_id, report_type, period_start)
);

-- +goose Down
-- Forward-only: released migrations are never rolled back.
