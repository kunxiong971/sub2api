-- 渠道监控：api_mode CHECK 约束放行 images
--
-- Images API 探活模式（api_mode='images'，见 243 之后的功能改动）需要
-- 数据库约束同步放行；此前约束仅允许 chat_completions/responses，
-- 保存 images 监控会触发 500（constraint violated）。
-- 幂等执行：可重复运行

ALTER TABLE channel_monitors DROP CONSTRAINT IF EXISTS channel_monitors_api_mode_check;

ALTER TABLE channel_monitors ADD CONSTRAINT channel_monitors_api_mode_check
    CHECK (api_mode IN ('chat_completions','responses','images'));
