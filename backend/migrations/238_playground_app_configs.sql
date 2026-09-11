-- 集成工作台（对话/生图/画布）「注入配置」表
--
-- 管理员按应用绑定分组与模型展示清单。这里只存策略规则（哪个分组、哪些模型、
-- 展示什么名称/价格），不存任何真实密钥：运行时才取该用户在该分组下的 key 注入。
--
-- 幂等执行：可重复运行

CREATE TABLE IF NOT EXISTS playground_app_configs (
    id BIGSERIAL PRIMARY KEY,
    app VARCHAR(32) NOT NULL,
    group_id BIGINT,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    inject_mode VARCHAR(32) NOT NULL DEFAULT '',
    sort INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_playground_app_configs_app
    ON playground_app_configs (app);

-- 应用下每个模型的展示配置（price_label / unit_hint 纯展示，不参与计费）
CREATE TABLE IF NOT EXISTS playground_app_models (
    id BIGSERIAL PRIMARY KEY,
    app_config_id BIGINT NOT NULL REFERENCES playground_app_configs(id) ON DELETE CASCADE,
    model_id VARCHAR(160) NOT NULL,
    display_name VARCHAR(160) NOT NULL DEFAULT '',
    price_label VARCHAR(160) NOT NULL DEFAULT '',
    unit_hint VARCHAR(160) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_playground_app_models_app_model
    ON playground_app_models (app_config_id, model_id);

CREATE INDEX IF NOT EXISTS idx_playground_app_models_sort
    ON playground_app_models (app_config_id, sort_order, id);
