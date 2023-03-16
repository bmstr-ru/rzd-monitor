drop table monitors;
create table monitors
(
    id                integer not null primary key,
    chat_id           integer not null,
    from_station_id   integer,
    from_station_name text,
    to_station_id     integer,
    to_station_name   text,
    departure_day     integer,
    departure_month   integer,
    departure_year    integer,
    train_number      text
);

create index monitors_chat_id_idx on monitors (chat_id);
create index monitors_departure_date_idx on monitors (departure_day, departure_month, departure_year);