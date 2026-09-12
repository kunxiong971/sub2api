-- 工作台配置：模型关联渠道监控
--
-- 每个注入模型可选关联一个渠道监控（channel_monitors.id）。
-- 用户端下发配置时，按关联监控的最近检测状态生成模型状态标签，
-- 让客户在工作台里直观看到「哪个模型当前可用」。
--
-- 关联是展示层的软引用：监控被删除时置空（ON DELETE SET NULL），不影响模型行。
-- 幂等执行：可重复运行

ALTER TABLE playground_app_models
    ADD COLUMN IF NOT EXISTS monitor_id BIGINT REFERENCES channel_monitors(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_playground_app_models_monitor
    ON playground_app_models (monitor_id)
    WHERE monitor_id IS NOT NULL;
