CREATE TABLE i_detail (
    i_id   VARCHAR(20),
    i_name VARCHAR(20)
);

INSERT INTO i_detail (i_id, i_name)
VALUES ('BH01', 'Bharat Index');

CREATE TABLE i_price (
    i_id    VARCHAR(20),
    i_ttime TIMESTAMP,
    i_price NUMERIC(12, 2)
);

CREATE TABLE s_detail (
    s_id   VARCHAR(20),
    s_name VARCHAR(20),
    s_min  INTEGER,
    s_max  INTEGER
);

INSERT INTO s_detail (s_id, s_name, s_min, s_max)
VALUES ('BT001', 'BATA', 20, 30),
       ('TT001', 'TATA', 40, 55);

CREATE TABLE s_price (
    s_id    VARCHAR(20),
    s_ttime TIMESTAMP,
    s_price INTEGER
);
