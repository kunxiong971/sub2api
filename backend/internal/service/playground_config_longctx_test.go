//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type longContextGroupRepoStub struct {
	GroupRepository
	group *Group
}

func (s *longContextGroupRepoStub) GetByID(context.Context, int64) (*Group, error) {
	return s.group, nil
}

func longContextTestGroup(enabled bool) *Group {
	return &Group{ID: 10, Name: "g", Platform: PlatformOpenAI, Status: StatusActive, LongContextPricingEnabled: enabled}
}

// 目录阶梯（fallback 价卡）路径：chat 模型下发三字段；image 模型（无 token 阶梯）不下发。
func TestListEnabledAppsEnrichesLongContextPricing(t *testing.T) {
	repo := &playgroundConfigRepoStub{
		globalCfgs: []PlaygroundGlobalConfig{{ID: 1, GroupID: 10, Enabled: true}},
		globalModels: map[int64][]PlaygroundAppModel{
			1: {
				{ModelID: "claude-sonnet-4", ModelKind: ModelKindChat, Enabled: true},
				{ModelID: "gpt-image-2", ModelKind: ModelKindImage, Enabled: true},
			},
		},
	}
	billing := &BillingService{fallbackPrices: map[string]*ModelPricing{
		"claude-sonnet-4": {
			InputPricePerToken: 3e-6, OutputPricePerToken: 15e-6,
			LongContextInputThreshold:     200000,
			LongContextThresholdInclusive: true,
			LongContextInputMultiplier:    2,
			LongContextOutputMultiplier:   2,
		},
	}}
	svc := NewPlaygroundConfigService(
		repo, nil,
		&longContextGroupRepoStub{group: longContextTestGroup(true)},
		nil,
		NewModelPricingResolver(nil, billing),
	)

	apps, err := svc.ListEnabledApps(context.Background())
	require.NoError(t, err)

	chat := apps[PlaygroundAppChat].Groups[0].Models
	require.Len(t, chat, 1)
	require.True(t, chat[0].LongContextPricingEnabled, "chat 模型需下发长上下文提醒字段")
	require.Equal(t, 200000, chat[0].LongContextThreshold)
	require.True(t, chat[0].LongContextThresholdInclusive)

	image := apps[PlaygroundAppImage].Groups[0].Models
	require.False(t, image[1].LongContextPricingEnabled, "非 chat 模型不富化")
	require.Zero(t, image[1].LongContextThreshold)
}

// 分组未启用长上下文计费：resolved.longContextPricingEnabled=false，字段保持零值。
func TestListEnabledAppsSkipsLongContextWhenGroupDisabled(t *testing.T) {
	repo := &playgroundConfigRepoStub{
		globalCfgs:   []PlaygroundGlobalConfig{{ID: 1, GroupID: 10, Enabled: true}},
		globalModels: map[int64][]PlaygroundAppModel{1: {{ModelID: "claude-sonnet-4", ModelKind: ModelKindChat, Enabled: true}}},
	}
	billing := &BillingService{fallbackPrices: map[string]*ModelPricing{
		"claude-sonnet-4": {LongContextInputThreshold: 200000, LongContextInputMultiplier: 2},
	}}
	svc := NewPlaygroundConfigService(
		repo, nil,
		&longContextGroupRepoStub{group: longContextTestGroup(false)},
		nil,
		NewModelPricingResolver(nil, billing),
	)

	apps, err := svc.ListEnabledApps(context.Background())
	require.NoError(t, err)
	m := apps[PlaygroundAppChat].Groups[0].Models[0]
	require.False(t, m.LongContextPricingEnabled)
	require.Zero(t, m.LongContextThreshold)
}

// 无 resolver（旧最小装配）与无阈值目录：字段零值，不阻塞下发。
func TestListEnabledAppsToleratesMissingResolverOrThreshold(t *testing.T) {
	repo := &playgroundConfigRepoStub{
		globalCfgs:   []PlaygroundGlobalConfig{{ID: 1, GroupID: 10, Enabled: true}},
		globalModels: map[int64][]PlaygroundAppModel{1: {{ModelID: "claude-sonnet-4", ModelKind: ModelKindChat, Enabled: true}}},
	}
	plain := NewPlaygroundConfigService(repo, nil, &longContextGroupRepoStub{group: longContextTestGroup(true)}, nil, nil)
	apps, err := plain.ListEnabledApps(context.Background())
	require.NoError(t, err)
	require.Zero(t, apps[PlaygroundAppChat].Groups[0].Models[0].LongContextThreshold)

	// 目录无长上下文阈值：零值
	billing := &BillingService{fallbackPrices: map[string]*ModelPricing{
		"claude-sonnet-4": {InputPricePerToken: 3e-6},
	}}
	withResolver := NewPlaygroundConfigService(repo, nil, &longContextGroupRepoStub{group: longContextTestGroup(true)}, nil, NewModelPricingResolver(nil, billing))
	apps2, err := withResolver.ListEnabledApps(context.Background())
	require.NoError(t, err)
	require.False(t, apps2[PlaygroundAppChat].Groups[0].Models[0].LongContextPricingEnabled)
}
