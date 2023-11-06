-- name: select-one
select 1;

-- name: test-table-ddl!
create table if not exists tests_data
(
    id   bigint unsigned primary key,
    name varchar(255)
);