-- goRAGENT 初始化 (MySQL 8.0)

CREATE TABLE IF NOT EXISTS t_user (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    username    VARCHAR(64)  NOT NULL UNIQUE,
    password    VARCHAR(256) NOT NULL,
    role        VARCHAR(16)  NOT NULL DEFAULT 'user',
    avatar      VARCHAR(512),
    deleted     TINYINT      NOT NULL DEFAULT 0,
    create_time DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_conversation (
    id              BIGINT AUTO_INCREMENT PRIMARY KEY,
    conversation_id VARCHAR(32) NOT NULL,
    user_id         VARCHAR(32) NOT NULL,
    title           VARCHAR(128),
    last_time       DATETIME,
    deleted         TINYINT     NOT NULL DEFAULT 0,
    create_time     DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time     DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_conv_user (conversation_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_conversation_message (
    id                BIGINT AUTO_INCREMENT PRIMARY KEY,
    conversation_id   VARCHAR(32)  NOT NULL,
    user_id           VARCHAR(32)  NOT NULL,
    role              VARCHAR(16)  NOT NULL,
    content           TEXT,
    thinking_content  TEXT,
    thinking_duration INT,
    vote              TINYINT,
    deleted           TINYINT      NOT NULL DEFAULT 0,
    create_time       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_msg_conv_user (conversation_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_conversation_summary (
    id              BIGINT AUTO_INCREMENT PRIMARY KEY,
    conversation_id VARCHAR(32) NOT NULL,
    user_id         VARCHAR(32) NOT NULL,
    content         TEXT,
    last_message_id VARCHAR(32),
    deleted         TINYINT     NOT NULL DEFAULT 0,
    create_time     DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time     DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_sum_conv_user (conversation_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_intent_node (
    id                    VARCHAR(32) PRIMARY KEY,
    kb_id                 VARCHAR(32),
    intent_code           VARCHAR(64)  NOT NULL UNIQUE,
    name                  VARCHAR(128) NOT NULL,
    level                 SMALLINT     NOT NULL,
    parent_code           VARCHAR(64),
    description           VARCHAR(512),
    examples              TEXT,
    collection_name       VARCHAR(128),
    top_k                 INT,
    mcp_tool_id           VARCHAR(128),
    kind                  SMALLINT     NOT NULL DEFAULT 0,
    prompt_snippet        TEXT,
    prompt_template       TEXT,
    param_prompt_template TEXT,
    sort_order            INT          NOT NULL DEFAULT 0,
    enabled               TINYINT      NOT NULL DEFAULT 1,
    create_by             VARCHAR(32),
    update_by             VARCHAR(32),
    create_time           DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time           DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted               TINYINT      NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_query_term_mapping (
    id          VARCHAR(32)  PRIMARY KEY,
    domain      VARCHAR(64),
    source_term VARCHAR(128) NOT NULL,
    target_term VARCHAR(128) NOT NULL,
    match_type  SMALLINT     NOT NULL DEFAULT 1,
    priority    INT          NOT NULL DEFAULT 100,
    enabled     TINYINT      NOT NULL DEFAULT 1,
    remark      VARCHAR(255),
    create_by   VARCHAR(32),
    update_by   VARCHAR(32),
    create_time DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted     TINYINT      NOT NULL DEFAULT 0,
    INDEX idx_qtm_source (source_term)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_knowledge_base (
    id              VARCHAR(32) PRIMARY KEY,
    name            VARCHAR(128) NOT NULL,
    description     TEXT,
    embedding_model VARCHAR(64),
    collection_name VARCHAR(128),
    dimension       INT          DEFAULT 1536,
    deleted         TINYINT      NOT NULL DEFAULT 0,
    create_time     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_document (
    id          VARCHAR(32)  PRIMARY KEY,
    kb_id       VARCHAR(32)  NOT NULL,
    file_name   VARCHAR(256) NOT NULL,
    file_type   VARCHAR(32),
    file_size   BIGINT,
    status      VARCHAR(32)  DEFAULT 'PENDING',
    chunk_count INT          DEFAULT 0,
    deleted     TINYINT      NOT NULL DEFAULT 0,
    create_time DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_rag_trace_run (
    id              BIGINT AUTO_INCREMENT PRIMARY KEY,
    run_id          VARCHAR(64)  NOT NULL UNIQUE,
    conversation_id VARCHAR(32),
    user_id         VARCHAR(32),
    question        TEXT,
    status          VARCHAR(16)  DEFAULT 'RUNNING',
    error_message   TEXT,
    create_time     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_rag_trace_node (
    id             BIGINT AUTO_INCREMENT PRIMARY KEY,
    run_id         VARCHAR(64)  NOT NULL,
    parent_node_id VARCHAR(64),
    node_name      VARCHAR(128) NOT NULL,
    node_type      VARCHAR(32),
    input          TEXT,
    output         TEXT,
    duration_ms    BIGINT,
    error_message  TEXT,
    create_time    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_chunk (
    id              VARCHAR(32)  PRIMARY KEY,
    doc_id          VARCHAR(32)  NOT NULL,
    kb_id           VARCHAR(32)  NOT NULL,
    chunk_index     INT          NOT NULL,
    text            MEDIUMTEXT   NOT NULL,
    char_count      INT          DEFAULT 0,
    token_count     INT          DEFAULT 0,
    embedding_status VARCHAR(16) DEFAULT 'PENDING',
    enabled         TINYINT      NOT NULL DEFAULT 1,
    deleted         TINYINT      NOT NULL DEFAULT 0,
    create_time     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_ingestion_task (
    id              BIGINT AUTO_INCREMENT PRIMARY KEY,
    kb_id           VARCHAR(32)  NOT NULL,
    doc_id          VARCHAR(32)  NOT NULL,
    status          VARCHAR(16)  DEFAULT 'PENDING',
    total_chunks    INT          DEFAULT 0,
    completed_chunks INT         DEFAULT 0,
    error_message   TEXT,
    create_time     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_biz_change_log (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    entity_type   VARCHAR(64)  NOT NULL COMMENT 'kb/document/chunk/intent/mapping/user',
    entity_id     VARCHAR(64)  NOT NULL,
    action        VARCHAR(32)  NOT NULL COMMENT 'CREATE/UPDATE/DELETE/ENABLE/DISABLE',
    operator      VARCHAR(64),
    before_snapshot TEXT,
    after_snapshot  TEXT,
    create_time   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_change_entity (entity_type, entity_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_sample_question (
    id          VARCHAR(32)  PRIMARY KEY,
    question    VARCHAR(512) NOT NULL,
    sort_order  INT          DEFAULT 0,
    enabled     TINYINT      NOT NULL DEFAULT 1,
    deleted     TINYINT      NOT NULL DEFAULT 0,
    create_time DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
