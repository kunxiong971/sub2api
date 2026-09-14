-- 渠道监控：图片模型探活开关
--
-- 新增 channel_monitors.image_mode 布尔列：
--   false（默认）- 维持原有文本判定（2xx + textPath 文本非空才算可用）
--   true         - 2xx + 原始响应体非空即算可用
--                  生图模型（gpt-image 系，Responses API）的响应只有
--                  image_generation_call 没有文本，按文本判定会永久误判 failed，
--                  无法监控图片渠道连通性；开启后按响应体判定。
-- 幂等执行：可重复运行

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS image_mode BOOLEAN NOT NULL DEFAULT FALSE;
