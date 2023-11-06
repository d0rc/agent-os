-- name: ddl-create-page-cache
CREATE TABLE if not exists `page_cache` (
      `id` bigint unsigned NOT NULL AUTO_INCREMENT,
      `url` varchar(768) NOT NULL,
      `raw_content` mediumblob NOT NULL,
      `created_at` datetime NOT NULL,
      `cache_hits` int unsigned NOT NULL,
      `status_code` int unsigned NOT NULL,
      PRIMARY KEY (`id`),
      KEY `url` (`url`)
) ROW_FORMAT=COMPRESSED;

-- name: query-page-cache
select id, url, raw_content, created_at, cache_hits, status_code from page_cache where url = ?;

-- name: save-page-cache-record
insert into page_cache (url, raw_content, created_at, cache_hits, status_code) values (?,?,?,?,?);

-- name: make-page-cache-hit
update page_cache set cache_hits = cache_hits + 1 where id = ?;

-- name: ddl-create-search-cache
create table if not exists search_cache (
    id bigint unsigned not null auto_increment,
    keywords varchar(1024),
    lang varchar(32),
    country varchar(32),
    location varchar(255),
    raw_content mediumblob not null,
    created_at datetime not null,
    cache_hits int unsigned not null,
    key idx_search_cache_keywords (keywords, lang, country, location),
    primary key (id));

-- name: query-search-by-keywords
select id, keywords, lang, country, location, raw_content, created_at, cache_hits from search_cache where keywords =? and lang =? and country =? and location =?;

-- name: save-search-cache-record
insert into search_cache (
          keywords,
          lang,
          country,
          location,
          raw_content,
          created_at,
          cache_hits)
values (?,?,?,?,?,?,?);

-- name: make-search-cache-hit
update search_cache set cache_hits = cache_hits + 1 where id = ?;

-- name: ddl-create-llm-cache
create table if not exists llm_cache (
    id bigint unsigned not null  auto_increment,
    model varchar(1024),
    prompt mediumblob not null,
    prompt_length int unsigned not null,
    created_at datetime not null,
    generation_settings varchar(1024),
    cache_hits int unsigned not null,
    generation_result mediumblob not null,
    primary key (id));

-- name: insert-llm-cache-record
insert into llm_cache (model, prompt, prompt_length, created_at, generation_settings, cache_hits, generation_result)
    values (?,?,?,?,?,?,?);

-- name: query-llm-cache-by-id
select id, model, prompt, prompt_length, created_at, generation_settings, cache_hits, generation_result from llm_cache where id = ?;

-- name: query-llm-cache
select id,
       model,
       prompt,
       prompt_length,
       created_at,
       generation_settings,
       cache_hits,
       generation_result
from llm_cache where
    prompt_length = ? and
    prompt = ?;

-- name: make-llm-cache-hit
update llm_cache set cache_hits = cache_hits + 1 where id = ?;

-- name: ddl-task-cache
create table if not exists compute_cache (
    id bigint unsigned not null auto_increment,
    namespace varchar(255),
    task_hash varchar(255),
    task_result longblob not null,
    unique(namespace, task_hash),
    primary key(id),
    cache_hits int unsigned not null default 0,
    created_at timestamp not null default current_timestamp
) ROW_FORMAT=COMPRESSED;

-- name: mark-task-cache-hit
update compute_cache set cache_hits = cache_hits + 1 where id = ?;

-- name: get-task-cache-record
select id,
       namespace,
       task_result
from compute_cache where namespace =? and task_hash =?;

-- name: save-task-cache-record
insert into compute_cache (namespace, task_hash, task_result) values (?,?,?);