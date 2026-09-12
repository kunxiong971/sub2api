-- 工作台配置：模型类型标记
--
-- 管理端可为每个注入模型显式标记类型：chat(对话) / image(生图) / video(视频) / audio(语音)。
-- 空串 = 未标记，工作台按模型名关键词自动推断（历史行为）。
-- 幂等执行：可重复运行

ALTER TABLE playground_app_models
    ADD COLUMN IF NOT EXISTS model_kind VARCHAR(20) NOT NULL DEFAULT '';
