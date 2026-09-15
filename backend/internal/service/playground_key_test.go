//go:build unit

package service

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Stubs：只实现托管 key 解析路径用到的方法，其余方法由嵌入的 nil 接口占位
// （被调用即 panic，说明测试走错了路径）。
// ---------------------------------------------------------------------------

type playgroundKeyRepoStub struct {
	APIKeyRepository

	mu          sync.Mutex
	byUserGroup map[string]*APIKey
	nextID      int64
	createCalls int
	// missLookups 让前 N 次精确查询假装未命中，用于模拟「另一进程刚创建」的并发场景。
	missLookups int
}

func newPlaygroundKeyRepoStub() *playgroundKeyRepoStub {
	return &playgroundKeyRepoStub{byUserGroup: map[string]*APIKey{}, nextID: 100}
}

func playgroundKeyUserGroupKey(userID, groupID int64) string {
	return strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(groupID, 10)
}

func (s *playgroundKeyRepoStub) GetManagedByUserAndGroup(_ context.Context, userID, groupID int64) (*APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.missLookups > 0 {
		s.missLookups--
		return nil, ErrAPIKeyNotFound
	}
	key, ok := s.byUserGroup[playgroundKeyUserGroupKey(userID, groupID)]
	if !ok {
		return nil, ErrAPIKeyNotFound
	}
	clone := *key
	return &clone, nil
}

func (s *playgroundKeyRepoStub) Create(_ context.Context, key *APIKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createCalls++
	if key.GroupID == nil {
		return nil
	}
	id := playgroundKeyUserGroupKey(key.UserID, *key.GroupID)
	if existing, ok := s.byUserGroup[id]; ok {
		// 模拟部分唯一索引冲突（api_keys_playground_managed_user_group_unique）
		key.ID = existing.ID
		return ErrAPIKeyExists
	}
	s.nextID++
	key.ID = s.nextID
	clone := *key
	s.byUserGroup[id] = &clone
	return nil
}

func (s *playgroundKeyRepoStub) SoftDeleteOrphanManagedKeys(context.Context) ([]string, int64, error) {
	panic("unexpected SoftDeleteOrphanManagedKeys call")
}

func (s *playgroundKeyRepoStub) CountManagedKeysByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected CountManagedKeysByGroupID call")
}

type playgroundUserRepoStub struct {
	UserRepository
	user *User
}

func (s *playgroundUserRepoStub) GetByID(context.Context, int64) (*User, error) {
	return s.user, nil
}

type playgroundGroupRepoStub struct {
	GroupRepository
	group *Group
	calls int
}

func (s *playgroundGroupRepoStub) GetByID(_ context.Context, id int64) (*Group, error) {
	s.calls++
	if s.group == nil {
		return nil, ErrGroupNotFound
	}
	clone := *s.group
	clone.ID = id
	return &clone, nil
}

func newPlaygroundKeyService(repo APIKeyRepository, groupRepo GroupRepository) *APIKeyService {
	return NewAPIKeyService(
		repo,
		&playgroundUserRepoStub{user: &User{ID: 7, Status: StatusActive}},
		groupRepo,
		nil,
		nil,
		nil,
		&config.Config{},
	)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// 并发注入（10 个请求）只应创建一把托管 key：singleflight 折叠同 uid+gid 的创建。
func TestEnsurePlaygroundKeyConcurrentInjectionCreatesSingleKey(t *testing.T) {
	repo := newPlaygroundKeyRepoStub()
	groupRepo := &playgroundGroupRepoStub{group: &Group{ID: 3, Name: "ZN-GPT", Status: StatusActive}}
	svc := newPlaygroundKeyService(repo, groupRepo)

	const workers = 10
	var wg sync.WaitGroup
	keys := make([]*APIKey, workers)
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			keys[idx], errs[idx] = svc.EnsurePlaygroundKey(context.Background(), 7, 3, "ZN-GPT")
		}(i)
	}
	wg.Wait()

	for i := 0; i < workers; i++ {
		require.NoError(t, errs[i])
		require.NotNil(t, keys[i])
		require.Equal(t, keys[0].ID, keys[i].ID, "并发调用必须返回同一把托管 key")
	}
	require.Equal(t, 1, repo.createCalls, "并发注入只应创建一把托管 key")
	require.Equal(t, PlaygroundKeyNamePrefix+"ZN-GPT", keys[0].Name)
}

// 已存在托管 key 时直接复用，不再创建（修复「列表窗口漏掉 → 重复创建」）。
func TestEnsurePlaygroundKeyReusesExistingKey(t *testing.T) {
	repo := newPlaygroundKeyRepoStub()
	groupID := int64(5)
	repo.byUserGroup[playgroundKeyUserGroupKey(7, groupID)] = &APIKey{
		ID:      42,
		UserID:  7,
		GroupID: &groupID,
		Name:    PlaygroundKeyNamePrefix + "既有分组",
		Key:     "sk-existing",
		Status:  StatusActive,
	}
	svc := newPlaygroundKeyService(repo, &playgroundGroupRepoStub{group: &Group{ID: groupID, Name: "既有分组"}})

	key, err := svc.EnsurePlaygroundKey(context.Background(), 7, groupID, "既有分组")

	require.NoError(t, err)
	require.Equal(t, int64(42), key.ID)
	require.Equal(t, "sk-existing", key.Key)
	require.Zero(t, repo.createCalls, "命中已有托管 key 时不应再创建")
}

// 跨进程并发：精确查询未命中后创建撞唯一索引，应回查已有记录返回，而不是报错。
func TestEnsurePlaygroundKeyFallsBackToExistingKeyOnUniqueConflict(t *testing.T) {
	repo := newPlaygroundKeyRepoStub()
	groupID := int64(9)
	// 另一实例已写入：本实例首次查询假装未命中，创建时撞唯一索引。
	repo.missLookups = 1
	repo.byUserGroup[playgroundKeyUserGroupKey(7, groupID)] = &APIKey{
		ID:      77,
		UserID:  7,
		GroupID: &groupID,
		Name:    PlaygroundKeyNamePrefix + "并发分组",
		Key:     "sk-concurrent",
		Status:  StatusActive,
	}
	svc := newPlaygroundKeyService(repo, &playgroundGroupRepoStub{group: &Group{ID: groupID, Name: "并发分组"}})

	key, err := svc.EnsurePlaygroundKey(context.Background(), 7, groupID, "并发分组")

	require.NoError(t, err)
	require.Equal(t, int64(77), key.ID, "唯一冲突后应返回另一实例创建的那把 key")
	require.Equal(t, 1, repo.createCalls)
}

// /pgw 入口只持有 groupID：分组名由服务层回查，命名保持 `Playground · <分组名>`。
func TestEnsurePlaygroundKeyResolvesGroupNameFromRepo(t *testing.T) {
	repo := newPlaygroundKeyRepoStub()
	groupRepo := &playgroundGroupRepoStub{group: &Group{ID: 11, Name: "C2-Image", Status: StatusActive}}
	svc := newPlaygroundKeyService(repo, groupRepo)

	key, err := svc.EnsurePlaygroundKey(context.Background(), 7, 11, "")

	require.NoError(t, err)
	require.Equal(t, PlaygroundKeyNamePrefix+"C2-Image", key.Name)
	// 一次用于解析命名，一次是 Create 内的分组权限校验
	require.NotZero(t, groupRepo.calls)
}

func TestPlaygroundKeyDisplayName(t *testing.T) {
	require.Equal(t, "Playground · ZN-GPT", PlaygroundKeyDisplayName("ZN-GPT", 3))
	require.Equal(t, "Playground · ZN-GPT", PlaygroundKeyDisplayName("  ZN-GPT  ", 3))
	require.Equal(t, "Playground · 分组 #12", PlaygroundKeyDisplayName("", 12))
	require.True(t, IsManagedPlaygroundKeyName(PlaygroundKeyDisplayName("", 12)))

	long := strings.Repeat("分", 200)
	name := PlaygroundKeyDisplayName(long, 1)
	require.Equal(t, playgroundKeyNameMaxRunes, len([]rune(name)), "超长分组名必须截断到 name 列上限")
	require.True(t, IsManagedPlaygroundKeyName(name))
}
