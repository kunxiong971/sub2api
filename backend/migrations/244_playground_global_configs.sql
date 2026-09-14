-- 全局渠道配置：把「工作台配置」的完整能力（添加渠道 → 拉取模型 → 配置展示信息）
-- 上提为全局一份，对话 / 生图 / 画布三个工作台按模型类型自动分流注入。
--
-- 结构：
--   playground_global_configs  全局渠道绑定（分组 × 启停 × 排序）
--   playground_global_models   全局渠道下的模型清单（展示名/价格/类型/监控…）
-- 注入规则（后端按 app 过滤 kind）：
--   chat   → chat
--   image  → chat + image（Agent 需要 LLM）
--   canvas → chat + image（文本节点需要 LLM）
--
-- 幂等执行：可重复运行

CREATE TABLE IF NOT EXISTS playground_global_configs (
    id         BIGSERIAL PRIMARY KEY,
    group_id   BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    sort       INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_playground_global_configs_group
    ON playground_global_configs (group_id);

-- 全局模型挂到渠道绑定下（同一 model_id 允许出现在多个渠道，展示信息各自独立）
ALTER TABLE playground_global_models
    ADD COLUMN IF NOT EXISTS global_config_id BIGINT REFERENCES playground_global_configs(id) ON DELETE CASCADE;

-- 先移除旧的「model_id 全局唯一」索引：同一模型即将允许挂到多个渠道，
-- 且必须在按渠道回填模型之前删除，否则回填 INSERT 会违反唯一约束。
DROP INDEX IF EXISTS idx_playground_global_models_model;

-- 1) 用既有各应用绑定去重生成全局渠道（组 2/3/5…），启停取「任一应用启用即启用」
INSERT INTO playground_global_configs (group_id, enabled, sort)
SELECT group_id, BOOL_OR(enabled), MIN(sort)
FROM playground_app_configs
GROUP BY group_id
ON CONFLICT (group_id) DO NOTHING;

-- 2) 把迁移 242 导入的扁平模型模板，按「该渠道账号真实支持的模型」回填到各渠道下：
--    账号 credentials.model_mapping 的键集合 = 该渠道实际可调用的模型，最贴近真实能力。
INSERT INTO playground_global_models
    (global_config_id, model_id, display_name, price_label, unit_hint, description, model_kind, enabled, sort_order, monitor_id)
SELECT DISTINCT ON (gc.id, gm.model_id)
    gc.id, gm.model_id, gm.display_name, gm.price_label, gm.unit_hint, gm.description,
    gm.model_kind, gm.enabled, gm.sort_order, gm.monitor_id
FROM playground_global_configs gc
JOIN account_groups ag ON ag.group_id = gc.group_id
JOIN accounts a ON a.id = ag.account_id AND a.deleted_at IS NULL
JOIN playground_global_models gm
    ON gm.global_config_id IS NULL
   AND gm.model_id <> ''
   AND (a.credentials::jsonb -> 'model_mapping') ? gm.model_id
ORDER BY gc.id, gm.model_id, gm.id;

-- 3) 移除已迁走的扁平模板行（global_config_id 为空的旧数据）
DELETE FROM playground_global_models WHERE global_config_id IS NULL;

-- 4) 约束调整：model_id 不再全局唯一（同一模型可挂多个渠道），改为渠道内唯一
CREATE UNIQUE INDEX IF NOT EXISTS idx_playground_global_models_binding_model
    ON playground_global_models (global_config_id, model_id);

CREATE INDEX IF NOT EXISTS idx_playground_global_models_binding
    ON playground_global_models (global_config_id, sort_order, id);

-- 5) 回填完成后收紧非空约束（新表写法，允许重复执行）
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'playground_global_models'
          AND column_name = 'global_config_id'
          AND is_nullable = 'YES'
    ) THEN
        ALTER TABLE playground_global_models ALTER COLUMN global_config_id SET NOT NULL;
    END IF;
END $$;
