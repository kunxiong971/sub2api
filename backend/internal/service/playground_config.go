package service

import (
	"context"
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
)

// PlaygroundKeyNamePrefix 集成工作台自动创建的 key 名称前缀。
const PlaygroundKeyNamePrefix = "Playground · "

// IsManagedPlaygroundKeyName 判断 key 名称是否属于工作台托管密钥。
func IsManagedPlaygroundKeyName(name string) bool {
	return strings.HasPrefix(name, PlaygroundKeyNamePrefix)
}

// PlaygroundAppModel 应用下单个模型的展示配置。
//
// DisplayName / PriceLabel / UnitHint 纯展示，不参与计费；
// 模型类型标记：管理端可为每个注入模型显式指定类型，工作台按类型分流注入
// （对话进 Chat、生图进 Images、视频/语音同理）；空串 = 未标记，按模型名关键词自动推断。
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
	// ModelKind 模型类型标记：chat/image/video/audio；空串 = 按模型名自动推断。
	ModelKind string `json:"model_kind,omitempty"`
	// MonitorID 可选关联的渠道监控（展示层软引用，仅用于下发状态标签）。
	MonitorID *int64 `json:"monitor_id,omitempty"`
	// MonitorStatus 关联监控的最近检测状态（下发时快照）：
	// operational / degraded / failed / error；空串 = 未关联或无检测数据。
	MonitorStatus string    `json:"monitor_status,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
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
	// ListGlobalModels 全局模型库：模型展示信息只维护一份，各工作台按类型自动注入。
	ListGlobalModels(ctx context.Context) ([]PlaygroundGlobalModel, error)
	// ReplaceGlobalModels 全量替换全局模型库，事务内完成。
	ReplaceGlobalModels(ctx context.Context, models []PlaygroundGlobalModel) error
}

// PlaygroundGlobalModel 全局模型库条目：维护一份展示信息，各工作台按类型自动注入。
type PlaygroundGlobalModel struct {
	ID          int64     `json:"id"`
	ModelID     string    `json:"model_id"`
	DisplayName string    `json:"display_name"`
	PriceLabel  string    `json:"price_label"`
	UnitHint    string    `json:"unit_hint"`
	Description string    `json:"description"`
	ModelKind   string    `json:"model_kind"`
	Enabled     bool      `json:"enabled"`
	SortOrder   int       `json:"sort_order"`
	MonitorID   *int64    `json:"monitor_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
}

// NewPlaygroundConfigService 创建工作台注入配置服务。
// monitorService 可为 nil（测试场景）：nil 时模型状态标签不下发。
func NewPlaygroundConfigService(
	repo PlaygroundConfigRepository,
	adminService AdminService,
	groupRepo GroupRepository,
	monitorService *ChannelMonitorService,
) *PlaygroundConfigService {
	return &PlaygroundConfigService{
		repo:           repo,
		adminService:   adminService,
		groupRepo:      groupRepo,
		monitorService: monitorService,
	}
}

// ListApps 返回三个应用的配置总览（每个应用含其绑定的多个分组）。
func (s *PlaygroundConfigService) ListApps(ctx context.Context) ([]PlaygroundAppBundle, error) {
	cfgs, err := s.repo.ListAppConfigs(ctx)
	if err != nil {
		return nil, err
	}
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
// fork: 模型来源 = 全局模型库（按 app 允许的类型过滤）∪ 应用层手工清单（历史数据，去重）。
// 全局库让模型展示信息只维护一份、各工作台按类型自动注入；应用层清单保留作精细覆盖。
func (s *PlaygroundConfigService) ListEnabledApps(ctx context.Context) (map[string]PlaygroundRuntimeApp, error) {
	cfgs, err := s.repo.ListAppConfigs(ctx)
	if err != nil {
		return nil, err
	}
	globalModels, gErr := s.repo.ListGlobalModels(ctx)
	if gErr != nil {
		return nil, gErr
	}

	byApp := make(map[string][]*PlaygroundAppConfig)
	for i := range cfgs {
		c := &cfgs[i]
		app := NormalizePlaygroundApp(c.App)
		if app == "" || !c.Enabled {
			continue
		}
		byApp[app] = append(byApp[app], c)
	}

	out := make(map[string]PlaygroundRuntimeApp, len(byApp))
	for app, appCfgs := range byApp {
		// 该应用允许注入的全局模型（按类型过滤；未标记全量注入，由工作台前端兜底分流）
		globalForApp := filterGlobalModelsForApp(globalModels, app)
		groups := make([]PlaygroundRuntimeGroup, 0, len(appCfgs))
		for _, c := range appCfgs {
			models, mErr := s.repo.ListModelsByAppConfig(ctx, c.ID)
			if mErr != nil {
				return nil, mErr
			}
			merged := mergeGlobalAndAppModels(globalForApp, models)
			s.enrichMonitorStatuses(ctx, merged)
			groups = append(groups, PlaygroundRuntimeGroup{
				GroupID: c.GroupID,
				Models:  merged,
			})
		}
		injectMode := DefaultPlaygroundInjectMode(app)
		out[app] = PlaygroundRuntimeApp{
			App:        app,
			InjectMode: injectMode,
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

// filterGlobalModelsForApp 把全局库按应用允许的类型过滤成运行时模型清单。
// 未标记（空 kind）的模型全量注入，由各工作台前端按模型名兜底分流。
func filterGlobalModelsForApp(all []PlaygroundGlobalModel, app string) []PlaygroundAppModel {
	allowed := playgroundAppAllowedKinds(app)
	out := make([]PlaygroundAppModel, 0, len(all))
	for _, g := range all {
		if !g.Enabled {
			continue
		}
		kind := normalizeModelKind(g.ModelKind)
		if kind != "" && !allowed[kind] {
			continue
		}
		out = append(out, PlaygroundAppModel{
			ModelID:     strings.TrimSpace(g.ModelID),
			DisplayName: strings.TrimSpace(g.DisplayName),
			PriceLabel:  strings.TrimSpace(g.PriceLabel),
			UnitHint:    strings.TrimSpace(g.UnitHint),
			Description: strings.TrimSpace(g.Description),
			Enabled:     true,
			SortOrder:   g.SortOrder,
			ModelKind:   kind,
			MonitorID:   g.MonitorID,
		})
	}
	return out
}

// mergeGlobalAndAppModels 合并全局库与应用层清单：按 model_id 去重，全局库优先。
func mergeGlobalAndAppModels(global []PlaygroundAppModel, appModels []PlaygroundAppModel) []PlaygroundAppModel {
	seen := make(map[string]struct{}, len(global)+len(appModels))
	out := make([]PlaygroundAppModel, 0, len(global)+len(appModels))
	for _, m := range global {
		if m.ModelID == "" {
			continue
		}
		if _, dup := seen[m.ModelID]; dup {
			continue
		}
		seen[m.ModelID] = struct{}{}
		out = append(out, m)
	}
	for _, m := range appModels {
		if !m.Enabled || strings.TrimSpace(m.ModelID) == "" {
			continue
		}
		if _, dup := seen[m.ModelID]; dup {
			continue
		}
		seen[m.ModelID] = struct{}{}
		out = append(out, m)
	}
	return out
}

// ListGlobalModels 管理端：全局模型库清单（按排序）。
func (s *PlaygroundConfigService) ListGlobalModels(ctx context.Context) ([]PlaygroundGlobalModel, error) {
	models, err := s.repo.ListGlobalModels(ctx)
	if err != nil {
		return nil, err
	}
	if models == nil {
		models = []PlaygroundGlobalModel{}
	}
	return models, nil
}

// UpdateGlobalModels 管理端：全量替换全局模型库（归一化 + 按 model_id 去重后写入）。
func (s *PlaygroundConfigService) UpdateGlobalModels(ctx context.Context, models []PlaygroundGlobalModel) ([]PlaygroundGlobalModel, error) {
	normalized := make([]PlaygroundGlobalModel, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for i := range models {
		m := models[i]
		m.ModelID = strings.TrimSpace(m.ModelID)
		if m.ModelID == "" {
			continue
		}
		if _, dup := seen[m.ModelID]; dup {
			continue
		}
		seen[m.ModelID] = struct{}{}
		m.DisplayName = strings.TrimSpace(m.DisplayName)
		m.PriceLabel = strings.TrimSpace(m.PriceLabel)
		m.UnitHint = strings.TrimSpace(m.UnitHint)
		m.Description = strings.TrimSpace(m.Description)
		m.ModelKind = normalizeModelKind(m.ModelKind)
		if m.MonitorID != nil && *m.MonitorID <= 0 {
			m.MonitorID = nil
		}
		normalized = append(normalized, m)
	}
	if err := s.repo.ReplaceGlobalModels(ctx, normalized); err != nil {
		return nil, err
	}
	return s.ListGlobalModels(ctx)
}

// normalizeModels 归一化模型清单：去空、去重、裁剪展示字段。
// 注意：MonitorID 仅做引用透传，不在归一化时校验存在性（监控被删由 DB 置空）。
func (s *PlaygroundConfigService) normalizeModels(models []PlaygroundAppModel) []PlaygroundAppModel {
	normalized := make([]PlaygroundAppModel, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, m := range models {
		m.ModelID = strings.TrimSpace(m.ModelID)
		if m.ModelID == "" {
			continue
		}
		if _, dup := seen[m.ModelID]; dup {
			continue
		}
		seen[m.ModelID] = struct{}{}
		m.DisplayName = strings.TrimSpace(m.DisplayName)
		m.PriceLabel = strings.TrimSpace(m.PriceLabel)
		m.UnitHint = strings.TrimSpace(m.UnitHint)
		m.Description = strings.TrimSpace(m.Description)
		if m.MonitorID != nil && *m.MonitorID <= 0 {
			m.MonitorID = nil
		}
		m.ModelKind = normalizeModelKind(m.ModelKind)
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
