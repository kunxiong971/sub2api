-- 全局模型库：模型展示信息只维护一份，各工作台按类型自动注入
--
-- model_kind：chat(对话) / image(生图) / video(视频) / audio(语音)；空 = 未标记
-- 注入规则（后端按 app 过滤）：
--   chat  工作台 → chat
--   image 工作台 → chat + image（Agent 需要 LLM）
--   canvas 工作台 → chat + image（文本节点需要 LLM）
--   未标记（空）→ 全量注入，由各工作台前端按模型名兜底推断
--
-- 幂等执行：可重复运行

CREATE TABLE IF NOT EXISTS playground_global_models (
    id          BIGSERIAL PRIMARY KEY,
    model_id    VARCHAR(160) NOT NULL,
    display_name VARCHAR(160) NOT NULL DEFAULT '',
    price_label VARCHAR(160) NOT NULL DEFAULT '',
    unit_hint   VARCHAR(160) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    model_kind  VARCHAR(20) NOT NULL DEFAULT '',
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order  INTEGER NOT NULL DEFAULT 0,
    monitor_id  BIGINT REFERENCES channel_monitors(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_playground_global_models_model
    ON playground_global_models (model_id);

-- 存量导入：把现有各应用手工添加的模型去重汇入全局库（每个 model_id 取展示信息最全的一条）
INSERT INTO playground_global_models
    (model_id, display_name, price_label, unit_hint, description, model_kind, monitor_id)
SELECT DISTINCT ON (model_id)
    model_id, display_name, price_label, unit_hint, description, model_kind, monitor_id
FROM playground_app_models
ORDER BY model_id,
    (display_name <> '') DESC,
    (price_label <> '') DESC,
    (model_kind <> '') DESC,
    id DESC
ON CONFLICT (model_id) DO NOTHING;
