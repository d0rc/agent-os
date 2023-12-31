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
    `id` bigint unsigned NOT NULL AUTO_INCREMENT,
    `keywords` varchar(1024) DEFAULT NULL,
    `lang` varchar(32) DEFAULT NULL,
    `country` varchar(32) DEFAULT NULL,
    `location` varchar(255) DEFAULT NULL,
    `raw_content` mediumblob NOT NULL,
    `created_at` datetime NOT NULL,
    `cache_hits` int unsigned NOT NULL,
    PRIMARY KEY (`id`));

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

-- name: make-search-cache-hits
update search_cache set cache_hits = cache_hits + 1 where id in (?);

-- name: ddl-create-llm-embeddings
CREATE TABLE  if not exists  `llm_embeddings` (
    `id` bigint unsigned NOT NULL AUTO_INCREMENT,
    `model` varchar(255) DEFAULT NULL,
    `namespace` varchar(255) DEFAULT NULL,
    `namespace_id` bigint unsigned NOT NULL,
    text_hash varchar(255) DEFAULT NULL,
    `embedding` mediumblob,
    dims bigint unsigned not null,
    cache_hits int unsigned default 0,
    PRIMARY KEY (`id`),
    KEY `lookup_key` (`model`,`namespace`,`namespace_id`),
    unique `text_hash` (`model`, `text_hash`)) ROW_FORMAT=COMPRESSED;

-- name: make-embeddings-cache-hit
update llm_embeddings set cache_hits = cache_hits + 1 where id = ?;

-- name: insert-embeddings-cache-record
insert into llm_embeddings (
    model,
    namespace,
    namespace_id,
    text_hash,
    dims,
    embedding) values (?,?,?,?,?,?) on duplicate key update embedding = values(embedding);

-- name: query-embeddings-cache
select id, model, namespace, namespace_id, text_hash, embedding from llm_embeddings where model = ? and text_hash = ?;

-- name: get-embeddings-by-id
select id, model, namespace, namespace_id, text_hash, embedding from llm_embeddings where id = ?;

-- name: get-embeddings-by-text
select id, model, namespace, namespace_id, text_hash, embedding from llm_embeddings where text_hash = ?;

-- name: ddl-embeddings-queues
create table  if not exists  embeddings_queues (
    id bigint unsigned not null auto_increment,
    queue_name varchar(255),
    queue_pointer bigint unsigned,
    primary key (id),
    unique (queue_name)
);

-- name: set-embeddings-queue-pointer
insert into embeddings_queues (queue_name, queue_pointer) values (?, ?) on duplicate key update queue_pointer = values(queue_pointer);

-- name: get-embeddings-queue-pointer
select id, queue_name, queue_pointer from embeddings_queues where queue_name = ?;

-- name: ddl-create-llm-cache
create table if not exists llm_cache (
    `id` bigint unsigned NOT NULL AUTO_INCREMENT,
    `model` varchar(1024) DEFAULT NULL,
    `prompt` mediumblob NOT NULL,
    `prompt_length` int unsigned NOT NULL,
    `created_at` datetime NOT NULL,
    `generation_settings` varchar(1024) DEFAULT NULL,
    `cache_hits` int unsigned NOT NULL,
    `generation_result` mediumblob NOT NULL,
    PRIMARY KEY (`id`),
    KEY `prompt_length` (`prompt_length`,`prompt`(900))
 );

-- name: insert-llm-cache-record
insert into llm_cache (model, prompt, prompt_length, created_at, generation_settings, cache_hits, generation_result)
    values (?,?,?,?,?,?,?);

-- name: query-llm-cache-by-id
select id, model, prompt, prompt_length, created_at, generation_settings, cache_hits, generation_result from llm_cache where id = ?;

-- name: query-llm-cache-by-ids-multi
select id, model, prompt, prompt_length, created_at, generation_settings, cache_hits, generation_result from llm_cache where id > ? order by id limit ?;

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

-- name: make-llm-cache-hits
update llm_cache set cache_hits = cache_hits + 1 where id in (?);

-- name: ddl-task-cache
create table if not exists compute_cache (
    `id` bigint unsigned NOT NULL AUTO_INCREMENT,
    `namespace` varchar(255) DEFAULT NULL,
    `task_hash` varchar(255) DEFAULT NULL,
    `task_result` longblob NOT NULL,
    `cache_hits` int unsigned NOT NULL DEFAULT '0',
    `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `namespace` (`namespace`,`task_hash`)
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

-- name: ddl-create-messages
CREATE TABLE if not exists `messages` (
    `id` varchar(255) NOT NULL,
    `role` varchar(255) DEFAULT NULL,
    `content` mediumtext,
    `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `name` varchar(255) DEFAULT NULL,
    PRIMARY KEY (`id`),
    KEY `role` (`role`),
    KEY `name` (`name`),
    FULLTEXT KEY `content` (`content`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=COMPRESSED ;

-- name: ddl-create-message-links
CREATE TABLE if not exists `message_links` (
     `id` varchar(255) NOT NULL,
     `reply_to` varchar(255) NOT NULL,
     KEY `id` (`id`),
     KEY `reply_to` (`reply_to`),
     unique (id, reply_to)
) ENGINE=InnoDB;

-- name: save-messages-trace
insert into messages (id, name, role, content) values (?, ?, ?, ?) on duplicate key update role=values(role), name=values(name);

-- name: save-message-link
insert into message_links (id, reply_to) values (?,?) on duplicate key update id=values(id);

-- name: get-message-by-id
select id, name, role, content from messages where id =?;

-- name: get-messages-by-ids
select id, name, role, content from messages where id in (?);

-- name: get-messages-by-text
select id, name, role, content from messages where content like ?;

-- name: get-message-links-by-id
select id, reply_to from message_links where id = ?;

-- name: get-message-links-by-reply-to
select id, reply_to from message_links where reply_to =?;

-- name: get-messages-links-by-reply-to
select id, reply_to from message_links where reply_to  in (?);

-- name: get-agent-roots
select id from messages where messages.name = ? and role="system";

-- name: get-paired-embeddings
select
    lle1.id as lle1_id,
    lle1.namespace as lle1_namespace,
    lle1.namespace_id as lle1_namespace_id,
    lle1.embedding as lle1_embedding,
    lle2.id as lle2_id,
    lle2.namespace as lle2_namespace,
    lle2.namespace_id as lle2_namespace_id,
    lle2.embedding as lle2_embedding
from llm_embeddings as lle1 left join llm_embeddings as lle2 on lle1.namespace_id = lle2.namespace_id
where
    lle2.namespace = "llm-cache-generation" and
    lle1.namespace = "llm-cache-prompt";