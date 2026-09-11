-- 工作台注入配置：一个应用支持绑定多个分组（多渠道）
--
-- 原模型是一个 app 唯一一行（app 唯一 + 单 group_id），现改为「一个 app 可绑定
-- 多个分组」，每个 (app, group_id) 一行，各自独立启停并各自维护模型清单。
--
-- 幂等执行：可重复运行

-- 旧语义下 group_id 可为空；新语义要求每个绑定必须有分组，先清理空绑定（当前无）
DELETE FROM playground_app_configs WHERE group_id IS NULL;

ALTER TABLE playground_app_configs
    ALTER COLUMN group_id SET NOT NULL;

DROP INDEX IF EXISTS idx_playground_app_configs_app;

CREATE UNIQUE INDEX IF NOT EXISTS idx_playground_app_configs_app_group
    ON playground_app_configs (app, group_id);
