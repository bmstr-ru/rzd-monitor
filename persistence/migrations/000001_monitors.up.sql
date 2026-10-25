BEGIN;

CREATE TABLE IF NOT EXISTS rzd_monitor.monitors (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    from_station_id TEXT NOT NULL,
    from_station_name TEXT NOT NULL,
    to_station_id TEXT NOT NULL,
    to_station_name TEXT NOT NULL,
    departure_date DATE NOT NULL,
    train_number TEXT NOT NULL,
    scheduled BOOLEAN NOT NULL DEFAULT TRUE
);
CREATE INDEX IF NOT EXISTS monitors_chat_id_idx ON rzd_monitor.monitors (chat_id);
CREATE INDEX IF NOT EXISTS monitors_scheduled_idx ON rzd_monitor.monitors (id) WHERE scheduled;
COMMIT;
