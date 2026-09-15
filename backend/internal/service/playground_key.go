package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// =============================================================================
// 工作台托管 key（Playground · 前缀）解析与创建
//
// 背景：集成工作台（对话 / 生图 / 画布）注入的凭据是「每个用户在每个分组下
// 一把托管 key」。历史实现从分页列表（最近 1000 条）里内存过滤，key 多时会
// 误判为「没有」并重复创建，同时 /pgw 入口还会用 `Group #N` 命名 → 同组出现
// 多把不同名的托管 key。
//
// 现在统一收敛到 EnsurePlaygroundKey：
//   1. 按 user_id + group_id 精确 SQL 查询（不再受列表窗口影响）；
//   2. 同进程内同 uid+gid 并发用 singleflight 折叠为一次创建；
//   3. 跨进程并发由部分唯一索引兜底（api_keys_playground_managed_user_group_unique），
//      命中唯一冲突时回查已有记录返回；
//   4. 命名统一为 `Playground · <分组名>`。
// =============================================================================

// ManagedAPIKeyRepository 是工作台托管 key 的可选仓储能力。
// 与 apiKeyAllByUserIDLister 同理：并非所有实现（测试替身）都提供，缺失时调用方
// 返回明确错误而不是静默降级。
type ManagedAPIKeyRepository interface {
	// GetManagedByUserAndGroup 精确查询用户在指定分组下的托管 key（不存在返回 ErrAPIKeyNotFound）。
	GetManagedByUserAndGroup(ctx context.Context, userID, groupID int64) (*APIKey, error)
	// SoftDeleteOrphanManagedKeys 软删孤儿托管 key：group_id 为空或指向已软删/不存在的分组。
	// 返回被清理的 key 值（供失效认证缓存）与数量。
	SoftDeleteOrphanManagedKeys(ctx context.Context) ([]string, int64, error)
	// CountManagedKeysByGroupID 统计指定分组下未删除的托管 key 数量。
	CountManagedKeysByGroupID(ctx context.Context, groupID int64) (int64, error)
}

// EnsurePlaygroundKey 返回用户在指定分组下的工作台托管 key；不存在则创建一把。
//
// groupName 为空时回查分组名（/pgw 代理入口只持有 groupID），保证命名统一。
// 返回的 key 一定属于该 (userID, groupID)，调用方可直接下发给工作台。
func (s *APIKeyService) EnsurePlaygroundKey(ctx context.Context, userID, groupID int64, groupName string) (*APIKey, error) {
	if s == nil || s.apiKeyRepo == nil {
		return nil, fmt.Errorf("api key repository is unavailable")
	}
	if userID <= 0 || groupID <= 0 {
		return nil, ErrAPIKeyNotFound
	}
	repo, ok := s.apiKeyRepo.(ManagedAPIKeyRepository)
	if !ok {
		return nil, fmt.Errorf("managed api key repository is unavailable")
	}

	// singleflight：同一进程内同 uid+gid 的并发请求只创建一把。
	flightKey := fmt.Sprintf("%d:%d", userID, groupID)
	value, err, _ := s.playgroundKeySF.Do(flightKey, func() (any, error) {
		existing, lookupErr := repo.GetManagedByUserAndGroup(ctx, userID, groupID)
		if lookupErr == nil && existing != nil {
			return existing, nil
		}
		if lookupErr != nil && !errors.Is(lookupErr, ErrAPIKeyNotFound) {
			return nil, lookupErr
		}

		created, createErr := s.Create(ctx, userID, CreateAPIKeyRequest{
			Name:    s.playgroundKeyDisplayName(ctx, groupID, groupName),
			GroupID: &groupID,
		})
		if createErr != nil {
			// 唯一索引冲突：并发（跨进程/跨实例）下已有记录，回查返回那一把。
			if errors.Is(createErr, ErrAPIKeyExists) {
				existing, retryErr := repo.GetManagedByUserAndGroup(ctx, userID, groupID)
				if retryErr == nil && existing != nil {
					return existing, nil
				}
			}
			return nil, createErr
		}
		return created, nil
	})
	if err != nil {
		return nil, err
	}
	key, _ := value.(*APIKey)
	if key == nil {
		return nil, ErrAPIKeyNotFound
	}
	return key, nil
}

// playgroundKeyDisplayName 解析托管 key 展示名：优先用调用方提供的分组名，
// 缺省时回查分组（失败则按分组 ID 兜底），保证同一分组下的命名稳定一致。
func (s *APIKeyService) playgroundKeyDisplayName(ctx context.Context, groupID int64, groupName string) string {
	if strings.TrimSpace(groupName) == "" && s.groupRepo != nil {
		if group, err := s.groupRepo.GetByID(ctx, groupID); err == nil && group != nil {
			groupName = group.Name
		}
	}
	return PlaygroundKeyDisplayName(groupName, groupID)
}
