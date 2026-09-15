-- 245_playground_managed_key_idempotency.sql
-- 工作台托管 key（name 前缀 'Playground · '）幂等化：
--   1) 清理存量重复：同一 user_id + group_id 的多把托管 key 只保留最早创建的一把，其余软删；
--   2) 建立部分唯一索引：保证后续每个用户在每个分组下最多一把托管 key。
-- 迁移幂等可重复执行（先清洗、再 IF NOT EXISTS 建索引）。

-- ============================================================================
-- 1. 清洗存量重复（保留 created_at 最早、其次 id 最小的一把）
-- ============================================================================
UPDATE api_keys k
SET deleted_at = NOW(), updated_at = NOW()
WHERE k.deleted_at IS NULL
  AND k.name LIKE 'Playground · %'
  AND k.group_id IS NOT NULL
  AND EXISTS (
      SELECT 1
      FROM api_keys keep
      WHERE keep.deleted_at IS NULL
        AND keep.name LIKE 'Playground · %'
        AND keep.user_id = k.user_id
        AND keep.group_id = k.group_id
        AND (keep.created_at, keep.id) < (k.created_at, k.id)
  );

-- ============================================================================
-- 2. 部分唯一索引（软删记录不占用；仅约束托管 key，用户自建 key 不受影响）
-- ============================================================================
CREATE UNIQUE INDEX IF NOT EXISTS api_keys_playground_managed_user_group_unique
    ON api_keys (user_id, group_id)
    WHERE deleted_at IS NULL AND name LIKE 'Playground · %';
