package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// playgroundConfigRepository 工作台注入配置仓储（raw SQL）。
//
// 该表结构由 SQL 迁移维护（SQL 迁移是 schema 的权威来源），因此这里不依赖
// ent 生成代码，避免引入一次全量代码生成。
type playgroundConfigRepository struct {
	db *sql.DB
}

// NewPlaygroundConfigRepository 创建工作台注入配置仓储。
func NewPlaygroundConfigRepository(db *sql.DB) service.PlaygroundConfigRepository {
	return &playgroundConfigRepository{db: db}
}

const playgroundAppConfigSelectColumns = `id, app, group_id, enabled, COALESCE(inject_mode, ''), sort, created_at, updated_at`

func scanPlaygroundAppConfig(scan func(dest ...any) error) (*service.PlaygroundAppConfig, error) {
	cfg := &service.PlaygroundAppConfig{}
	if err := scan(
		&cfg.ID,
		&cfg.App,
		&cfg.GroupID,
		&cfg.Enabled,
		&cfg.InjectMode,
		&cfg.Sort,
		&cfg.CreatedAt,
		&cfg.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (r *playgroundConfigRepository) ListAppConfigs(ctx context.Context) ([]service.PlaygroundAppConfig, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil playground config repository")
	}
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+playgroundAppConfigSelectColumns+"\nFROM playground_app_configs\nORDER BY app, sort, id")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	cfgs := make([]service.PlaygroundAppConfig, 0)
	for rows.Next() {
		cfg, err := scanPlaygroundAppConfig(rows.Scan)
		if err != nil {
			return nil, err
		}
		cfgs = append(cfgs, *cfg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cfgs, nil
}

const playgroundAppModelSelectColumns = `id, app_config_id, model_id, display_name, price_label,
unit_hint, description, enabled, sort_order, model_kind, monitor_id, created_at, updated_at`

func scanPlaygroundAppModel(scan func(dest ...any) error) (*service.PlaygroundAppModel, error) {
	m := &service.PlaygroundAppModel{}
	if err := scan(
		&m.ID,
		&m.AppConfigID,
		&m.ModelID,
		&m.DisplayName,
		&m.PriceLabel,
		&m.UnitHint,
		&m.Description,
		&m.Enabled,
		&m.SortOrder,
		&m.ModelKind,
		&m.MonitorID,
		&m.CreatedAt,
		&m.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return m, nil
}

func (r *playgroundConfigRepository) ListModelsByAppConfig(ctx context.Context, appConfigID int64) ([]service.PlaygroundAppModel, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil playground config repository")
	}
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+playgroundAppModelSelectColumns+"\nFROM playground_app_models\nWHERE app_config_id = $1\nORDER BY sort_order, id",
		appConfigID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	models := make([]service.PlaygroundAppModel, 0)
	for rows.Next() {
		m, err := scanPlaygroundAppModel(rows.Scan)
		if err != nil {
			return nil, err
		}
		models = append(models, *m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return models, nil
}

// ReplaceApp 全量替换某应用的所有绑定与模型清单（事务内：先删后插）。
func (r *playgroundConfigRepository) ReplaceApp(ctx context.Context, app string, cfgs []service.PlaygroundAppConfig) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil playground config repository")
	}
	app = strings.TrimSpace(app)
	if app == "" {
		return fmt.Errorf("playground app is required")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, "DELETE FROM playground_app_configs WHERE app = $1", app); err != nil {
		return err
	}

	for i := range cfgs {
		c := &cfgs[i]
		var cfgID int64
		if err := tx.QueryRowContext(ctx, `
INSERT INTO playground_app_configs (app, group_id, enabled, inject_mode, sort, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
RETURNING id`,
			app, c.GroupID, c.Enabled, strings.TrimSpace(c.InjectMode), c.Sort,
		).Scan(&cfgID); err != nil {
			return err
		}

		for _, m := range c.Models {
			if strings.TrimSpace(m.ModelID) == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO playground_app_models
  (app_config_id, model_id, display_name, price_label, unit_hint, description, enabled, sort_order, model_kind, monitor_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())`,
				cfgID,
				truncateString(m.ModelID, 160),
				truncateString(m.DisplayName, 160),
				truncateString(m.PriceLabel, 160),
				truncateString(m.UnitHint, 160),
				strings.TrimSpace(m.Description),
				m.Enabled,
				m.SortOrder,
				m.ModelKind,
				m.MonitorID,
			); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

const playgroundGlobalConfigSelectColumns = `id, group_id, enabled, sort, created_at, updated_at`

func scanPlaygroundGlobalConfig(scan func(dest ...any) error) (*service.PlaygroundGlobalConfig, error) {
	c := &service.PlaygroundGlobalConfig{}
	if err := scan(
		&c.ID,
		&c.GroupID,
		&c.Enabled,
		&c.Sort,
		&c.CreatedAt,
		&c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return c, nil
}

// 全局模型行：global_config_id 复用 PlaygroundAppModel.AppConfigID 字段承载（语义同「所属绑定」）。
const playgroundGlobalModelSelectColumns = `id, global_config_id, model_id, display_name, price_label,
unit_hint, description, model_kind, enabled, sort_order, monitor_id, created_at, updated_at`

func scanPlaygroundGlobalModel(scan func(dest ...any) error) (*service.PlaygroundAppModel, error) {
	m := &service.PlaygroundAppModel{}
	if err := scan(
		&m.ID,
		&m.AppConfigID,
		&m.ModelID,
		&m.DisplayName,
		&m.PriceLabel,
		&m.UnitHint,
		&m.Description,
		&m.ModelKind,
		&m.Enabled,
		&m.SortOrder,
		&m.MonitorID,
		&m.CreatedAt,
		&m.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return m, nil
}

// ListGlobalConfigs 全局渠道配置（渠道绑定 + 各渠道下的模型清单，按排序）。
func (r *playgroundConfigRepository) ListGlobalConfigs(ctx context.Context) ([]service.PlaygroundGlobalConfig, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil playground config repository")
	}
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+playgroundGlobalConfigSelectColumns+"\nFROM playground_global_configs\nORDER BY sort, id")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	cfgs := make([]service.PlaygroundGlobalConfig, 0)
	for rows.Next() {
		c, err := scanPlaygroundGlobalConfig(rows.Scan)
		if err != nil {
			return nil, err
		}
		cfgs = append(cfgs, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(cfgs) == 0 {
		return cfgs, nil
	}

	modelRows, err := r.db.QueryContext(ctx,
		"SELECT "+playgroundGlobalModelSelectColumns+"\nFROM playground_global_models\nORDER BY sort_order, id")
	if err != nil {
		return nil, err
	}
	defer func() { _ = modelRows.Close() }()

	byConfig := make(map[int64][]service.PlaygroundAppModel, len(cfgs))
	for modelRows.Next() {
		m, err := scanPlaygroundGlobalModel(modelRows.Scan)
		if err != nil {
			return nil, err
		}
		byConfig[m.AppConfigID] = append(byConfig[m.AppConfigID], *m)
	}
	if err := modelRows.Err(); err != nil {
		return nil, err
	}
	for i := range cfgs {
		cfgs[i].Models = byConfig[cfgs[i].ID]
	}
	return cfgs, nil
}

// ReplaceGlobalConfigs 全量替换全局渠道配置（事务内：先删后插；模型随绑定级联删除）。
func (r *playgroundConfigRepository) ReplaceGlobalConfigs(ctx context.Context, cfgs []service.PlaygroundGlobalConfig) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil playground config repository")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, "DELETE FROM playground_global_configs"); err != nil {
		return err
	}

	for i := range cfgs {
		c := &cfgs[i]
		var cfgID int64
		if err := tx.QueryRowContext(ctx, `
INSERT INTO playground_global_configs (group_id, enabled, sort, created_at, updated_at)
VALUES ($1, $2, $3, NOW(), NOW())
RETURNING id`,
			c.GroupID, c.Enabled, c.Sort,
		).Scan(&cfgID); err != nil {
			return err
		}

		for _, m := range c.Models {
			if strings.TrimSpace(m.ModelID) == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO playground_global_models
  (global_config_id, model_id, display_name, price_label, unit_hint, description, model_kind, enabled, sort_order, monitor_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())`,
				cfgID,
				truncateString(m.ModelID, 160),
				truncateString(m.DisplayName, 160),
				truncateString(m.PriceLabel, 160),
				truncateString(m.UnitHint, 160),
				strings.TrimSpace(m.Description),
				truncateString(m.ModelKind, 20),
				m.Enabled,
				m.SortOrder,
				m.MonitorID,
			); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}
