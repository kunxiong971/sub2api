-- 切换为「全局模型库」驱动注入：清空各应用下的历史手工模型清单
--
-- 背景：迁移 242 已把原有手工清单按 model_id 去重汇入 playground_global_models；
-- 此后模型展示信息统一在全局库维护、按类型自动注入（chat→chat；image/canvas→chat+image）。
-- 保留每个「应用 × 分组」绑定关系（playground_app_configs）不变。
--
-- 注意：app 层清单仍受支持（手动补录的场景会与全局库合并注入，全局库优先），
-- 本迁移只清理历史遗留数据，不影响后续手动配置能力。
-- 幂等执行：可重复运行

DELETE FROM playground_app_models;
