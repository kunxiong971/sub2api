package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 集成工作台应用标识（固定三个，与面板壳页面路由 /chat /image /canvas 一一对应）
const (
	PlaygroundAppChat   = "chat"
	PlaygroundAppImage  = "image"
	PlaygroundAppCanvas = "canvas"
)

// PlaygroundSupportedApps 支持注入配置的应用列表，顺序即管理页与下发的默认顺序。
var PlaygroundSupportedApps = []string{PlaygroundAppChat, PlaygroundAppImage, PlaygroundAppCanvas}

// 注入方式：对话/画布走 postMessage，生图走 URL 查询参数。
const (
	PlaygroundInjectPostMessage = "postMessage"
	PlaygroundInjectURLParams   = "url"
)

// DefaultPlaygroundInjectMode 返回应用的默认注入方式。
func DefaultPlaygroundInjectMode(app string) string {
	if app == PlaygroundAppImage {
		return PlaygroundInjectURLParams
	}
	return PlaygroundInjectPostMessage
}

// NormalizePlaygroundApp 归一化应用标识，非法值返回空字符串。
func NormalizePlaygroundApp(app string) string {
	app = strings.TrimSpace(app)
	for _, supported := range PlaygroundSupportedApps {
		if app == supported {
			return supported
		}
	}
	return ""
}

var (
	// ErrPlaygroundAppUnsupported 应用标识不是 chat/image/canvas 之一。
	ErrPlaygroundAppUnsupported = infraerrors.BadRequest("PLAYGROUND_APP_UNSUPPORTED", "unsupported playground app")
	// ErrPlaygroundGroupNotFound 绑定的分组不存在。
	ErrPlaygroundGroupNotFound = infraerrors.BadRequest("PLAYGROUND_GROUP_NOT_FOUND", "playground group not found")
	// ErrPlaygroundModelRequired 模型 ID 为空。
	ErrPlaygroundModelRequired = infraerrors.BadRequest("PLAYGROUND_MODEL_REQUIRED", "model id is required")
	// ErrPlaygroundKeyManaged 工作台托管密钥不允许用户删除/编辑（阶段 C 锁死）。
	ErrPlaygroundKeyManaged = infraerrors.Forbidden("PLAYGROUND_KEY_MANAGED", "工作台托管密钥不可删除或编辑")
	// ErrPlaygroundKeyNotManaged 管理端托管 key 专用接口只接受 Playground 前缀的 key。
	ErrPlaygroundKeyNotManaged = infraerrors.BadRequest("PLAYGROUND_KEY_NOT_MANAGED", "该密钥不是工作台托管密钥")
)

// PlaygroundKeyNamePrefix 集成工作台自动创建的 key 名称前缀。
const PlaygroundKeyNamePrefix = "Playground · "

// playgroundKeyNameMaxRunes api_keys.name 列上限（varchar(100)，按字符计）。
const playgroundKeyNameMaxRunes = 100

// IsManagedPlaygroundKeyName 判断 key 名称是否属于工作台托管密钥。
func IsManagedPlaygroundKeyName(name string) bool {
	return strings.HasPrefix(name, PlaygroundKeyNamePrefix)
}

// PlaygroundKeyDisplayName 生成工作台托管 key 的统一展示名：`Playground · <分组名>`。
// 所有创建入口（注入配置接口 / /pgw 代理）都必须经此命名，避免同组出现
// `Playground · Group #N` 与 `Playground · <分组名>` 两把不同名的密钥。
// 分组名缺失时回退为「分组 #<id>」；超长时截断，保证不超过 api_keys.name 上限。
func PlaygroundKeyDisplayName(groupName string, groupID int64) string {
	name := strings.TrimSpace(groupName)
	if name == "" {
		name = fmt.Sprintf("分组 #%d", groupID)
	}
	limit := playgroundKeyNameMaxRunes - len([]rune(PlaygroundKeyNamePrefix))
	runes := []rune(name)
	if limit > 0 && len(runes) > limit {
		runes = runes[:limit]
	}
	return PlaygroundKeyNamePrefix + string(runes)
}

// PlaygroundAppModel 应用下单个模型的展示配置。
//
// DisplayName / PriceLabel / UnitHint 纯展示，不参与计费；
// 模型类型标记：管理端可为每个注入模型显式指定类型，工作台按类型分流注入
// （对话进 Chat、生图进 Images、视频/语音同理）；空串 = 保存时按模型名关键词
// 自动填充（InferModelKind），存量空值在下发时兜底推断。
const (
	ModelKindChat  = "chat"
	ModelKindImage = "image"
	ModelKindVideo = "video"
	ModelKindAudio = "audio"
)

// 计费一律按 sub2api 既有的分组倍率与计价体系。
type PlaygroundAppModel struct {
	ID          int64     `json:"id"`
	AppConfigID int64     `json:"app_config_id"`
	ModelID     string    `json:"model_id"`
	DisplayName string    `json:"display_name"`
	PriceLabel  string    `json:"price_label"`
	UnitHint    string    `json:"unit_hint"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	SortOrder   int       `json:"sort_order"`
	// ModelKind 模型类型标记：chat/image/video/audio；空串 = 保存时按 InferModelKind 自动填充。
	ModelKind string `json:"model_kind,omitempty"`
	// MonitorID 可选关联的渠道监控（展示层软引用，仅用于下发状态标签）。
	MonitorID *int64 `json:"monitor_id,omitempty"`
	// MonitorStatus 关联监控的最近检测状态（下发时快照）：
	// operational / degraded / failed / error；空串 = 未关联或无检测数据。
	MonitorStatus string    `json:"monitor_status,omitempty"`
	// —— 长上下文计费提醒（运行时富化字段，不落库：阈值随定价配置变）——
	// LongContextPricingEnabled 该「分组×模型」当前是否启用长上下文阶梯计费。
	LongContextPricingEnabled bool `json:"long_context_pricing_enabled,omitempty"`
	// LongContextThreshold 首次跳档的上下文 token 阈值（0 = 无）。
	LongContextThreshold int `json:"long_context_threshold,omitempty"`
	// LongContextThresholdInclusive true = 达到阈值即跳档（xAI 口径）；false = 严格大于。
	LongContextThresholdInclusive bool `json:"long_context_threshold_inclusive,omitempty"`
	CreatedAt                     time.Time `json:"created_at"`
	UpdatedAt                     time.Time `json:"updated_at"`
}

// PlaygroundAppConfig 单个「应用 × 分组」绑定。
//
// 一个应用可绑定多个分组（多渠道）；每个绑定独立启停、独立维护模型清单。
// 这里只保存策略（哪个分组、哪些模型、展示什么），不保存任何真实密钥。
type PlaygroundAppConfig struct {
	ID         int64
	App        string
	GroupID    int64
	Enabled    bool
	InjectMode string
	Sort       int
	CreatedAt  time.Time
	UpdatedAt  time.Time
	// Models 仅用于全量替换写入时携带该绑定下的模型清单，不映射为主表字段。
	Models []PlaygroundAppModel
}

// PlaygroundConfigRepository 工作台注入配置的数据访问接口。
type PlaygroundConfigRepository interface {
	ListAppConfigs(ctx context.Context) ([]PlaygroundAppConfig, error)
	ListModelsByAppConfig(ctx context.Context, appConfigID int64) ([]PlaygroundAppModel, error)
	// ReplaceApp 全量替换某应用的所有绑定（含每个绑定下的模型清单），事务内完成。
	ReplaceApp(ctx context.Context, app string, cfgs []PlaygroundAppConfig) error
	// ListGlobalConfigs 全局渠道配置：渠道绑定 + 各渠道下的模型清单。
	ListGlobalConfigs(ctx context.Context) ([]PlaygroundGlobalConfig, error)
	// ReplaceGlobalConfigs 全量替换全局渠道配置（绑定与模型），事务内完成。
	ReplaceGlobalConfigs(ctx context.Context, cfgs []PlaygroundGlobalConfig) error
}

// PlaygroundGlobalConfig 全局渠道绑定：一处维护「渠道 + 模型清单」，
// 对话 / 生图 / 画布三个工作台按模型类型自动分流注入。
type PlaygroundGlobalConfig struct {
	ID        int64
	GroupID   int64
	Enabled   bool
	Sort      int
	CreatedAt time.Time
	UpdatedAt time.Time
	// Models 该渠道下的模型清单（全量替换写入时携带，不映射为主表字段）。
	Models []PlaygroundAppModel
}

// PlaygroundGlobalBindingView 管理端：全局渠道 + 模型清单。
type PlaygroundGlobalBindingView struct {
	GroupID       int64                `json:"group_id"`
	GroupName     string               `json:"group_name"`
	GroupPlatform string               `json:"group_platform"`
	Enabled       bool                 `json:"enabled"`
	Models        []PlaygroundAppModel `json:"models"`
}

// PlaygroundBindingView 管理端总览：单个分组绑定 + 模型展示清单。
type PlaygroundBindingView struct {
	GroupID       int64                `json:"group_id"`
	GroupName     string               `json:"group_name"`
	GroupPlatform string               `json:"group_platform"`
	Enabled       bool                 `json:"enabled"`
	Models        []PlaygroundAppModel `json:"models"`
}

// PlaygroundAppBundle 管理端应用总览：应用 + 其绑定的多个分组。
type PlaygroundAppBundle struct {
	App        string                  `json:"app"`
	InjectMode string                  `json:"inject_mode"`
	Bindings   []PlaygroundBindingView `json:"bindings"`
}

// PlaygroundRuntimeGroup 用户端下发：单个分组 + 其启用的模型清单。
type PlaygroundRuntimeGroup struct {
	GroupID int64
	Models  []PlaygroundAppModel
}

// PlaygroundRuntimeApp 用户端下发：应用 + 其绑定的多个分组。
type PlaygroundRuntimeApp struct {
	App        string
	InjectMode string
	Groups     []PlaygroundRuntimeGroup
}

// PlaygroundConfigService 工作台注入配置服务。
type PlaygroundConfigService struct {
	repo           PlaygroundConfigRepository
	adminService   AdminService
	groupRepo      GroupRepository
	monitorService *ChannelMonitorService
	// pricingResolver 可选（测试/最小装配可为 nil，nil 时不下发长上下文字段）：
	// 下发时按「分组×模型」解析长上下文计费阈值，供对话工作台做临界双倍计费提醒。
	pricingResolver *ModelPricingResolver
}

// NewPlaygroundConfigService 创建工作台注入配置服务。
// monitorService / pricingResolver 均可为 nil（测试场景）：
// nil monitorService 时模型状态标签不下发；nil pricingResolver 时长上下文阈值不下发。
func NewPlaygroundConfigService(
	repo PlaygroundConfigRepository,
	adminService AdminService,
	groupRepo GroupRepository,
	monitorService *ChannelMonitorService,
	pricingResolver *ModelPricingResolver,
) *PlaygroundConfigService {
	return &PlaygroundConfigService{
		repo:            repo,
		adminService:    adminService,
		groupRepo:       groupRepo,
		monitorService:  monitorService,
		pricingResolver: pricingResolver,
	}
}

// groupExists 判断分组是否仍存在（未软删）。groupRepo 缺省（部分测试装配）时
// 一律视为存在，不启用僵尸绑定过滤。
// 背景：分组删除时未同步清理注入配置，留下指向已删分组的“僵尸绑定”——
// 会导致管理端保存被 ErrPlaygroundGroupNotFound 整单拒绝、运行时向死分组路由。
func (s *PlaygroundConfigService) groupExists(ctx context.Context, groupID int64, cache map[int64]bool) bool {
	if s.groupRepo == nil {
		return true
	}
	if v, ok := cache[groupID]; ok {
		return v
	}
	exists := false
	if _, err := s.groupRepo.GetByID(ctx, groupID); err == nil {
		exists = true
	}
	cache[groupID] = exists
	return exists
}

// ListApps 返回三个应用的配置总览（每个应用含其绑定的多个分组）。
func (s *PlaygroundConfigService) ListApps(ctx context.Context) ([]PlaygroundAppBundle, error) {
	cfgs, err := s.repo.ListAppConfigs(ctx)
	if err != nil {
		return nil, err
	}
	// fork: 过滤指向已删除分组的僵尸绑定，管理端不再出现幽灵分组
	existsCache := make(map[int64]bool)
	kept := make([]PlaygroundAppConfig, 0, len(cfgs))
	for i := range cfgs {
		if s.groupExists(ctx, cfgs[i].GroupID, existsCache) {
			kept = append(kept, cfgs[i])
		}
	}
	cfgs = kept
	groupNames, groupPlatforms := s.groupDisplayIndex(ctx)

	bundles := make([]PlaygroundAppBundle, 0, len(PlaygroundSupportedApps))
	for _, app := range PlaygroundSupportedApps {
		bundle := PlaygroundAppBundle{
			App:        app,
			InjectMode: DefaultPlaygroundInjectMode(app),
			Bindings:   []PlaygroundBindingView{},
		}
		for i := range cfgs {
			c := &cfgs[i]
			if NormalizePlaygroundApp(c.App) != app {
				continue
			}
			models, mErr := s.repo.ListModelsByAppConfig(ctx, c.ID)
			if mErr != nil {
				return nil, mErr
			}
			if models == nil {
				models = []PlaygroundAppModel{}
			}
			bundle.Bindings = append(bundle.Bindings, PlaygroundBindingView{
				GroupID:       c.GroupID,
				GroupName:     groupNames[c.GroupID],
				GroupPlatform: groupPlatforms[c.GroupID],
				Enabled:       c.Enabled,
				Models:        models,
			})
		}
		bundles = append(bundles, bundle)
	}
	return bundles, nil
}

// PlaygroundBindingInput 保存时单个分组绑定的输入。
type PlaygroundBindingInput struct {
	GroupID int64
	Enabled bool
	Models  []PlaygroundAppModel
}

// SaveApp 全量替换某应用的绑定分组（每个分组独立启停、独立模型清单）。
func (s *PlaygroundConfigService) SaveApp(ctx context.Context, app string, bindings []PlaygroundBindingInput) error {
	app = NormalizePlaygroundApp(app)
	if app == "" {
		return ErrPlaygroundAppUnsupported
	}

	cfgs := make([]PlaygroundAppConfig, 0, len(bindings))
	seen := make(map[int64]struct{}, len(bindings))
	for i := range bindings {
		b := &bindings[i]
		if b.GroupID <= 0 {
			continue
		}
		if _, err := s.groupRepo.GetByID(ctx, b.GroupID); err != nil {
			return ErrPlaygroundGroupNotFound
		}
		if _, dup := seen[b.GroupID]; dup {
			continue
		}
		seen[b.GroupID] = struct{}{}
		cfgs = append(cfgs, PlaygroundAppConfig{
			App:        app,
			GroupID:    b.GroupID,
			Enabled:    b.Enabled,
			InjectMode: DefaultPlaygroundInjectMode(app),
			Sort:       i,
			Models:     s.normalizeModels(b.Models),
		})
	}
	return s.repo.ReplaceApp(ctx, app, cfgs)
}

// GetModelCandidates 拉取某分组可选模型清单（复用分组候选，避免手填）。
func (s *PlaygroundConfigService) GetModelCandidates(ctx context.Context, groupID int64) ([]string, error) {
	if groupID <= 0 {
		return []string{}, nil
	}
	models, err := s.adminService.GetGroupModelsListCandidates(ctx, groupID, "")
	if err != nil {
		return nil, err
	}
	if models == nil {
		models = []string{}
	}
	return models, nil
}

// ListEnabledApps 返回当前启用且绑定分组的所有应用配置（按 app 索引）。
//
// fork: 模型来源 = 全局渠道配置（一处维护「渠道 + 模型」，按 app 允许的类型过滤分流）
// ∪ 应用层手工清单（历史补录/单独覆盖，全局优先）。
func (s *PlaygroundConfigService) ListEnabledApps(ctx context.Context) (map[string]PlaygroundRuntimeApp, error) {
	globalCfgs, err := s.repo.ListGlobalConfigs(ctx)
	if err != nil {
		return nil, err
	}
	cfgs, err := s.repo.ListAppConfigs(ctx)
	if err != nil {
		return nil, err
	}

	// fork: 过滤指向已删除分组的僵尸绑定（历史遗留：分组删除未同步清理注入配置），
	// 避免运行时向死分组路由；管理端同样按此过滤，保存不再被整单拒绝。
	existsCache := make(map[int64]bool)
	keptGlobal := make([]PlaygroundGlobalConfig, 0, len(globalCfgs))
	for i := range globalCfgs {
		if s.groupExists(ctx, globalCfgs[i].GroupID, existsCache) {
			keptGlobal = append(keptGlobal, globalCfgs[i])
		}
	}
	globalCfgs = keptGlobal

	// 应用层手工清单索引：app → group_id → models（仅作全局配置之外的补录）
	appLayer := make(map[string]map[int64][]PlaygroundAppModel)
	for i := range cfgs {
		c := &cfgs[i]
		app := NormalizePlaygroundApp(c.App)
		if app == "" || !c.Enabled {
			continue
		}
		if !s.groupExists(ctx, c.GroupID, existsCache) {
			continue
		}
		models, mErr := s.repo.ListModelsByAppConfig(ctx, c.ID)
		if mErr != nil {
			return nil, mErr
		}
		if appLayer[app] == nil {
			appLayer[app] = make(map[int64][]PlaygroundAppModel)
		}
		appLayer[app][c.GroupID] = models
	}

	out := make(map[string]PlaygroundRuntimeApp, len(PlaygroundSupportedApps))
	// fork: 长上下文阈值富化用的分组缓存（同一分组被 chat/image/canvas 三个应用复用，只查一次）
	groupCache := make(map[int64]*Group)
	for _, app := range PlaygroundSupportedApps {
		allowed := playgroundAppAllowedKinds(app)
		groups := make([]PlaygroundRuntimeGroup, 0, len(globalCfgs))
		seen := make(map[int64]struct{}, len(globalCfgs))

		// 1) 全局渠道：按该应用允许的模型类型过滤后注入
		for i := range globalCfgs {
			gc := &globalCfgs[i]
			if !gc.Enabled {
				continue
			}
			injected := filterModelsByAllowedKinds(gc.Models, allowed)
			merged := mergeGlobalAndAppModels(injected, appLayer[app][gc.GroupID])
			if len(merged) == 0 {
				continue
			}
			s.enrichMonitorStatuses(ctx, merged)
			s.enrichLongContextPricing(ctx, gc.GroupID, merged, groupCache)
			groups = append(groups, PlaygroundRuntimeGroup{GroupID: gc.GroupID, Models: merged})
			seen[gc.GroupID] = struct{}{}
		}

		// 2) 兼容：仅存在于应用层绑定、尚未纳入全局配置的渠道
		// fork: 历史补录清单同样按类型严格分流（空 kind 走 InferModelKind 推断、
		// 显式 kind 优先），与全局库口径一致，防止空 kind 图片模型混入对话台。
		for groupID, models := range appLayer[app] {
			if _, ok := seen[groupID]; ok {
				continue
			}
			injected := filterModelsByAllowedKinds(models, allowed)
			if len(injected) == 0 {
				continue
			}
			s.enrichMonitorStatuses(ctx, injected)
			s.enrichLongContextPricing(ctx, groupID, injected, groupCache)
			groups = append(groups, PlaygroundRuntimeGroup{GroupID: groupID, Models: injected})
		}

		out[app] = PlaygroundRuntimeApp{
			App:        app,
			InjectMode: DefaultPlaygroundInjectMode(app),
			Groups:     groups,
		}
	}
	return out, nil
}

// playgroundAppAllowedKinds 各工作台允许注入的模型类型（「需要什么注入什么」）：
//   - chat（对话）      → chat
//   - image（生图）     → chat + image（Agent 模式需要 LLM）
//   - canvas（画布）    → chat + image（文本节点需要 LLM）
func playgroundAppAllowedKinds(app string) map[string]bool {
	switch NormalizePlaygroundApp(app) {
	case "chat":
		return map[string]bool{ModelKindChat: true}
	case "image", "canvas":
		return map[string]bool{ModelKindChat: true, ModelKindImage: true}
	default:
		return map[string]bool{}
	}
}

// filterModelsByAllowedKinds 按应用允许的类型过滤渠道模型（剔除停用项，保持原顺序）。
// fork: 存量空 kind 的模型先走 InferModelKind 统一推断再过滤，防历史数据绕过分流
// （如空 kind 的 gpt-image-2 不得混进对话台/lobe 注入清单，也不得漏出生图台）；
// 显式指定的 kind 优先，不受关键词推断影响。
func filterModelsByAllowedKinds(models []PlaygroundAppModel, allowed map[string]bool) []PlaygroundAppModel {
	out := make([]PlaygroundAppModel, 0, len(models))
	for _, m := range models {
		if !m.Enabled {
			continue
		}
		kind := normalizeModelKind(m.ModelKind)
		if kind == "" {
			kind = InferModelKind(m.ModelID)
		}
		if !allowed[kind] {
			continue
		}
		m.ModelKind = kind
		out = append(out, m)
	}
	return out
}

// mergeGlobalAndAppModels 合并全局库与应用层清单：按 model_id 去重（fork: 大小写不敏感，
// GPT-Image-2 与 gpt-image-2 视为同一模型，保留一条），全局库优先。
func mergeGlobalAndAppModels(global []PlaygroundAppModel, appModels []PlaygroundAppModel) []PlaygroundAppModel {
	seen := make(map[string]struct{}, len(global)+len(appModels))
	out := make([]PlaygroundAppModel, 0, len(global)+len(appModels))
	for _, m := range global {
		if strings.TrimSpace(m.ModelID) == "" {
			continue
		}
		key := strings.ToLower(m.ModelID)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, m)
	}
	for _, m := range appModels {
		if !m.Enabled || strings.TrimSpace(m.ModelID) == "" {
			continue
		}
		key := strings.ToLower(m.ModelID)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, m)
	}
	return out
}

// ListGlobalConfig 管理端：全局渠道配置总览（渠道 + 模型清单）。
func (s *PlaygroundConfigService) ListGlobalConfig(ctx context.Context) ([]PlaygroundGlobalBindingView, error) {
	cfgs, err := s.repo.ListGlobalConfigs(ctx)
	if err != nil {
		return nil, err
	}
	// fork: 过滤指向已删除分组的僵尸绑定（历史遗留），管理端不再出现幽灵分组、
	// 全量保存也不会再被 ErrPlaygroundGroupNotFound 整单拒绝。
	existsCache := make(map[int64]bool)
	kept := make([]PlaygroundGlobalConfig, 0, len(cfgs))
	for i := range cfgs {
		if s.groupExists(ctx, cfgs[i].GroupID, existsCache) {
			kept = append(kept, cfgs[i])
		}
	}
	cfgs = kept
	groupNames, groupPlatforms := s.groupDisplayIndex(ctx)
	views := make([]PlaygroundGlobalBindingView, 0, len(cfgs))
	for i := range cfgs {
		c := &cfgs[i]
		models := c.Models
		if models == nil {
			models = []PlaygroundAppModel{}
		}
		views = append(views, PlaygroundGlobalBindingView{
			GroupID:       c.GroupID,
			GroupName:     groupNames[c.GroupID],
			GroupPlatform: groupPlatforms[c.GroupID],
			Enabled:       c.Enabled,
			Models:        models,
		})
	}
	return views, nil
}

// SaveGlobalConfig 管理端：全量替换全局渠道配置（渠道绑定 + 各渠道模型清单）。
func (s *PlaygroundConfigService) SaveGlobalConfig(ctx context.Context, bindings []PlaygroundBindingInput) error {
	cfgs := make([]PlaygroundGlobalConfig, 0, len(bindings))
	seen := make(map[int64]struct{}, len(bindings))
	for i := range bindings {
		b := &bindings[i]
		if b.GroupID <= 0 {
			continue
		}
		if s.groupRepo != nil {
			if _, err := s.groupRepo.GetByID(ctx, b.GroupID); err != nil {
				return ErrPlaygroundGroupNotFound
			}
		}
		if _, dup := seen[b.GroupID]; dup {
			continue
		}
		seen[b.GroupID] = struct{}{}
		cfgs = append(cfgs, PlaygroundGlobalConfig{
			GroupID: b.GroupID,
			Enabled: b.Enabled,
			Sort:    i,
			Models:  s.normalizeModels(b.Models),
		})
	}
	return s.repo.ReplaceGlobalConfigs(ctx, cfgs)
}

// normalizeModels 归一化模型清单：去空、去重（fork: 大小写不敏感）、裁剪展示字段。
// 注意：MonitorID 仅做引用透传，不在归一化时校验存在性（监控被删由 DB 置空）。
// fork: kind 为空时用 InferModelKind 计算后落库（保存时自动填充），
// 显式指定的 kind 优先，不覆盖。
func (s *PlaygroundConfigService) normalizeModels(models []PlaygroundAppModel) []PlaygroundAppModel {
	normalized := make([]PlaygroundAppModel, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, m := range models {
		m.ModelID = strings.TrimSpace(m.ModelID)
		if m.ModelID == "" {
			continue
		}
		key := strings.ToLower(m.ModelID)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		m.DisplayName = strings.TrimSpace(m.DisplayName)
		m.PriceLabel = strings.TrimSpace(m.PriceLabel)
		m.UnitHint = strings.TrimSpace(m.UnitHint)
		m.Description = strings.TrimSpace(m.Description)
		if m.MonitorID != nil && *m.MonitorID <= 0 {
			m.MonitorID = nil
		}
		m.ModelKind = normalizeModelKind(m.ModelKind)
		if m.ModelKind == "" {
			m.ModelKind = InferModelKind(m.ModelID)
		}
		normalized = append(normalized, m)
	}
	return normalized
}

// normalizeModelKind 归一模型类型标记：仅接受 chat/image/video/audio，其余归空（自动推断）。
func normalizeModelKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case ModelKindChat, ModelKindImage, ModelKindVideo, ModelKindAudio:
		return strings.TrimSpace(kind)
	default:
		return ""
	}
}

// enrichMonitorStatuses 为关联了渠道监控的模型填充最近检测状态（monitor_status）。
// 状态在配置下发时快照；监控功能关闭/非 v1 模式/查询失败时保持为空（前端不展示标签）。
func (s *PlaygroundConfigService) enrichMonitorStatuses(ctx context.Context, models []PlaygroundAppModel) {
	needed := make([]int64, 0, len(models))
	for i := range models {
		if models[i].MonitorID != nil {
			needed = append(needed, *models[i].MonitorID)
		}
	}
	if len(needed) == 0 {
		return
	}
	index := s.monitorService.PlaygroundStatusIndex(ctx)
	if len(index) == 0 {
		return
	}
	for i := range models {
		if models[i].MonitorID == nil {
			continue
		}
		models[i].MonitorStatus = ResolveMonitorModelStatus(index[*models[i].MonitorID], models[i].ModelID)
	}
}

// groupDisplayIndex 构建分组展示信息索引，供管理页显示分组名与平台。
func (s *PlaygroundConfigService) groupDisplayIndex(ctx context.Context) (map[int64]string, map[int64]string) {
	names := make(map[int64]string)
	platforms := make(map[int64]string)
	if s.groupRepo == nil {
		return names, platforms
	}
	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return names, platforms
	}
	for i := range groups {
		names[groups[i].ID] = groups[i].Name
		platforms[groups[i].ID] = groups[i].Platform
	}
	return names, platforms
}

// enrichLongContextPricing 为对话模型解析长上下文计费阈值（long_context_* 三字段）。
//
// 与计费同源：resolver.Resolve（分组卡 → 渠道 → 目录 → 策略）；轻量路径不跑计费
// 探针——无渠道区间时直接读目录/分组阶梯阈值（LongContextInputThreshold），
// 有渠道区间时取首个 MinTokens>0 的边界。仅对 model_kind=chat 的模型富化
// （image/video/audio 不走 token 阶梯计费）。resolver 缺省、分组查询失败、
// 非 token 计费、未启用长上下文或无阈值时字段保持零值，前端据此不渲染提醒。
// 注意：区间匹配为左开右闭 (min, max]（FindMatchingInterval），实际计费在
// token > MinTokens 时才进档；下发按 inclusive=true 以 MinTokens 提醒，
// 较真实跳档点提前 1 token，宁早勿晚。
func (s *PlaygroundConfigService) enrichLongContextPricing(
	ctx context.Context,
	groupID int64,
	models []PlaygroundAppModel,
	groupCache map[int64]*Group,
) {
	if s.pricingResolver == nil || s.groupRepo == nil || len(models) == 0 {
		return
	}
	group, ok := groupCache[groupID]
	if !ok {
		g, err := s.groupRepo.GetByID(ctx, groupID)
		// 查询失败不阻塞下发：字段留零值，前端无提醒
		if err != nil || g == nil {
			groupCache[groupID] = nil
			return
		}
		group = g
		groupCache[groupID] = group
	}
	if group == nil {
		return
	}
	gid := group.ID
	for i := range models {
		m := &models[i]
		kind := m.ModelKind
		if kind == "" {
			kind = InferModelKind(m.ModelID)
		}
		if kind != ModelKindChat {
			continue
		}
		resolved := s.pricingResolver.Resolve(ctx, PricingInput{Model: m.ModelID, Group: group, GroupID: &gid})
		// 非 token 计费（按次/图片/视频）不适用长上下文阶梯
		if resolved == nil || (resolved.Mode != "" && resolved.Mode != BillingModeToken) {
			continue
		}
		if !resolved.longContextPricingEnabled {
			continue
		}
		if len(resolved.Intervals) > 0 {
			for j := range resolved.Intervals {
				if resolved.Intervals[j].MinTokens > 0 {
					m.LongContextPricingEnabled = true
					m.LongContextThreshold = resolved.Intervals[j].MinTokens
					m.LongContextThresholdInclusive = true
					break
				}
			}
			continue
		}
		pricing := s.pricingResolver.GetIntervalPricing(resolved, 1)
		if pricing == nil || pricing.LongContextInputThreshold <= 0 {
			continue
		}
		m.LongContextPricingEnabled = true
		m.LongContextThreshold = pricing.LongContextInputThreshold
		m.LongContextThresholdInclusive = pricing.LongContextThresholdInclusive
	}
}

// ---------------------------------------------------------------------------
// 注入自检（管理端）：一眼核对三个工作台实际会收到的注入清单
// ---------------------------------------------------------------------------

// PlaygroundSelfCheckModel 注入自检的单模型视图（含运行时富化字段，不含任何密钥）。
type PlaygroundSelfCheckModel struct {
	ModelID                       string `json:"model_id"`
	DisplayName                   string `json:"display_name"`
	ModelKind                     string `json:"model_kind"`
	MonitorStatus                 string `json:"monitor_status"`
	LongContextPricingEnabled     bool   `json:"long_context_pricing_enabled"`
	LongContextThreshold          int    `json:"long_context_threshold,omitempty"`
	LongContextThresholdInclusive bool   `json:"long_context_threshold_inclusive,omitempty"`
}

// PlaygroundSelfCheckGroup 注入自检的单分组视图。
type PlaygroundSelfCheckGroup struct {
	GroupID   int64                      `json:"group_id"`
	GroupName string                     `json:"group_name"`
	Models    []PlaygroundSelfCheckModel `json:"models"`
}

// PlaygroundSelfCheckApp 注入自检的单应用视图。
type PlaygroundSelfCheckApp struct {
	App    string                     `json:"app"`
	Groups []PlaygroundSelfCheckGroup `json:"groups"`
}

// SelfCheck 管理端「注入自检」：复用 ListEnabledApps 的运行时组装（类型分流、
// 监控状态、长上下文阈值富化），返回三个工作台实际会收到的注入清单（不含任何
// 密钥），供管理员一次看清注入结果，无需逐个打开工作台验证。
func (s *PlaygroundConfigService) SelfCheck(ctx context.Context) ([]PlaygroundSelfCheckApp, error) {
	apps, err := s.ListEnabledApps(ctx)
	if err != nil {
		return nil, err
	}
	groupNames, _ := s.groupDisplayIndex(ctx)
	out := make([]PlaygroundSelfCheckApp, 0, len(PlaygroundSupportedApps))
	for _, app := range PlaygroundSupportedApps {
		ra := apps[app]
		groups := make([]PlaygroundSelfCheckGroup, 0, len(ra.Groups))
		for _, g := range ra.Groups {
			models := make([]PlaygroundSelfCheckModel, 0, len(g.Models))
			for _, m := range g.Models {
				models = append(models, PlaygroundSelfCheckModel{
					ModelID:                       m.ModelID,
					DisplayName:                   m.DisplayName,
					ModelKind:                     m.ModelKind,
					MonitorStatus:                 m.MonitorStatus,
					LongContextPricingEnabled:     m.LongContextPricingEnabled,
					LongContextThreshold:          m.LongContextThreshold,
					LongContextThresholdInclusive: m.LongContextThresholdInclusive,
				})
			}
			groups = append(groups, PlaygroundSelfCheckGroup{
				GroupID:   g.GroupID,
				GroupName: groupNames[g.GroupID],
				Models:    models,
			})
		}
		out = append(out, PlaygroundSelfCheckApp{App: app, Groups: groups})
	}
	return out, nil
}
