//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Stub：只实现 ListEnabledApps 用到的三个读方法，其余由嵌入的 nil 接口占位。
// ---------------------------------------------------------------------------

type playgroundConfigRepoStub struct {
	PlaygroundConfigRepository

	globalCfgs   []PlaygroundGlobalConfig
	globalModels map[int64][]PlaygroundAppModel
	appCfgs      []PlaygroundAppConfig
	appModels    map[int64][]PlaygroundAppModel
}

func (s *playgroundConfigRepoStub) ListGlobalConfigs(context.Context) ([]PlaygroundGlobalConfig, error) {
	out := make([]PlaygroundGlobalConfig, len(s.globalCfgs))
	copy(out, s.globalCfgs)
	for i := range out {
		out[i].Models = s.globalModels[out[i].ID]
	}
	return out, nil
}

func (s *playgroundConfigRepoStub) ListAppConfigs(context.Context) ([]PlaygroundAppConfig, error) {
	return s.appCfgs, nil
}

func (s *playgroundConfigRepoStub) ListModelsByAppConfig(_ context.Context, appConfigID int64) ([]PlaygroundAppModel, error) {
	return s.appModels[appConfigID], nil
}

func collectPlaygroundModelIDs(groups []PlaygroundRuntimeGroup) []string {
	out := make([]string, 0)
	for _, g := range groups {
		for _, m := range g.Models {
			out = append(out, m.ModelID)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// InferModelKind 关键词表：各类关键词、大小写不敏感、匹配模型名任意位置、默认 chat。
func TestInferModelKindKeywords(t *testing.T) {
	cases := []struct {
		modelID string
		want    string
	}{
		// image
		{"gpt-image-2", ModelKindImage},
		{"GPT-IMAGE-2", ModelKindImage},
		{"Dall-E-3", ModelKindImage},
		{"dall_e_2", ModelKindImage},
		{"FLUX.1-schnell", ModelKindImage},
		{"foo-SDXL-bar", ModelKindImage},
		{"stable-diffusion-3.5", ModelKindImage},
		{"midjourney-v6", ModelKindImage},
		{"mj-relax", ModelKindImage},
		{"gemini-2.0-flash-exp-image-generation", ModelKindImage},
		// video
		{"Sora-2", ModelKindVideo},
		{"veo-3", ModelKindVideo},
		{"kling-v2-master", ModelKindVideo},
		{"runway-gen3", ModelKindVideo},
		{"wanx2.1-t2v", ModelKindVideo},
		{"hunyuan-video", ModelKindVideo},
		{"seedance-1-pro", ModelKindVideo},
		// audio
		{"TTS-1", ModelKindAudio},
		{"whisper-1", ModelKindAudio},
		{"gpt-4o-audio-preview", ModelKindAudio},
		{"Speech-02", ModelKindAudio},
		{"voice-clone", ModelKindAudio},
		{"suno-v4", ModelKindAudio},
		// chat（兜底）
		{"gpt-4o", ModelKindChat},
		{"claude-3-5-sonnet", ModelKindChat},
		{" deepseek-chat ", ModelKindChat},
		{"", ModelKindChat},
	}
	for _, c := range cases {
		require.Equal(t, c.want, InferModelKind(c.modelID), "modelID=%q", c.modelID)
	}
}

// 空 kind 的图片名模型按 image 分流：不进对话台、不漏出生图台；显式 chat 优先。
func TestFilterModelsByAllowedKindsInfersEmptyKind(t *testing.T) {
	chatAllowed := playgroundAppAllowedKinds("chat")
	imageAllowed := playgroundAppAllowedKinds("image")

	models := []PlaygroundAppModel{
		{ModelID: "gpt-image-2", Enabled: true},                       // 空 kind 图片名 → 推断 image
		{ModelID: "dall-e-3", ModelKind: ModelKindChat, Enabled: true}, // 显式 chat，优先于关键词
		{ModelID: "gpt-4o", Enabled: true},                            // 空 kind 普通名 → chat
		{ModelID: "sora-2", Enabled: true},                            // 空 kind 视频名 → video，chat/image 台都不放行
		{ModelID: "flux-schnell", Enabled: false},                     // 停用剔除
	}

	chat := filterModelsByAllowedKinds(models, chatAllowed)
	require.ElementsMatch(t, []string{"dall-e-3", "gpt-4o"}, collectPlaygroundModelIDs(
		[]PlaygroundRuntimeGroup{{Models: chat}},
	), "空 kind 的图片名模型不得混入对话台")
	for _, m := range chat {
		require.NotEqual(t, "gpt-image-2", m.ModelID)
	}

	image := filterModelsByAllowedKinds(models, imageAllowed)
	require.ElementsMatch(t, []string{"gpt-image-2", "dall-e-3", "gpt-4o"}, collectPlaygroundModelIDs(
		[]PlaygroundRuntimeGroup{{Models: image}},
	), "空 kind 的图片名模型不得漏出生图台")
	for _, m := range image {
		if m.ModelID == "gpt-image-2" {
			require.Equal(t, ModelKindImage, m.ModelKind, "推断出的 kind 需回填到下发结构")
		}
		if m.ModelID == "dall-e-3" {
			require.Equal(t, ModelKindChat, m.ModelKind, "显式指定的 chat 不被关键词推断覆盖")
		}
	}
}

// 保存归一化：空 kind 自动填充、显式 kind 保留、大小写不敏感去重、空 ID 剔除。
func TestNormalizeModelsFillsKindAndDedupesCaseInsensitive(t *testing.T) {
	svc := &PlaygroundConfigService{}
	out := svc.normalizeModels([]PlaygroundAppModel{
		{ModelID: "GPT-Image-2", ModelKind: " ", Enabled: true},
		{ModelID: "gpt-image-2", Enabled: true},                          // 与上一条仅大小写不同 → 重复
		{ModelID: " dall-e-3 ", ModelKind: ModelKindChat, Enabled: true}, // 显式 chat 优先
		{ModelID: "sora-2", ModelKind: "bogus", Enabled: true},           // 非法 kind 归空后按名推断
		{ModelID: "   ", Enabled: true},                                  // 空 ID 剔除
	})
	require.Len(t, out, 3)
	require.Equal(t, "GPT-Image-2", out[0].ModelID)
	require.Equal(t, ModelKindImage, out[0].ModelKind, "空 kind 保存时按模型名自动填充")
	require.Equal(t, "dall-e-3", out[1].ModelID)
	require.Equal(t, ModelKindChat, out[1].ModelKind, "显式指定的 kind 优先，不被推断覆盖")
	require.Equal(t, "sora-2", out[2].ModelID)
	require.Equal(t, ModelKindVideo, out[2].ModelKind)
}

// 合并去重改为大小写不敏感：GPT-Image-2 与 gpt-image-2 视为重复，全局库优先。
func TestMergeGlobalAndAppModelsCaseInsensitiveDedup(t *testing.T) {
	merged := mergeGlobalAndAppModels(
		[]PlaygroundAppModel{{ModelID: "GPT-Image-2", Enabled: true}},
		[]PlaygroundAppModel{
			{ModelID: "gpt-image-2", Enabled: true},
			{ModelID: "claude-3-5-sonnet", Enabled: true},
			{ModelID: "disabled-model", Enabled: false},
		},
	)
	require.Len(t, merged, 2)
	require.Equal(t, "GPT-Image-2", merged[0].ModelID, "全局库优先保留")
	require.Equal(t, "claude-3-5-sonnet", merged[1].ModelID)
}

// 下发链路端到端：存量空 kind 图片名模型严格按 image 分流（含应用层历史补录路径）。
func TestListEnabledAppsRoutesLegacyEmptyKindModels(t *testing.T) {
	repo := &playgroundConfigRepoStub{
		globalCfgs: []PlaygroundGlobalConfig{{ID: 1, GroupID: 10, Enabled: true}},
		globalModels: map[int64][]PlaygroundAppModel{
			1: {
				{ModelID: "gpt-image-2", Enabled: true},                        // 存量空 kind
				{ModelID: "gpt-4o", Enabled: true},
				{ModelID: "dall-e-3", ModelKind: ModelKindChat, Enabled: true}, // 显式 chat
			},
		},
		appCfgs: []PlaygroundAppConfig{
			{ID: 2, App: PlaygroundAppChat, GroupID: 20, Enabled: true},
		},
		appModels: map[int64][]PlaygroundAppModel{
			2: {{ModelID: "flux-schnell", Enabled: true}}, // 应用层历史补录，空 kind 图片名
		},
	}
	svc := NewPlaygroundConfigService(repo, nil, nil, nil, nil)

	apps, err := svc.ListEnabledApps(context.Background())
	require.NoError(t, err)

	require.ElementsMatch(t, []string{"gpt-4o", "dall-e-3"},
		collectPlaygroundModelIDs(apps[PlaygroundAppChat].Groups),
		"空 kind 的图片名模型不得出现在对话台/lobe 注入清单（含应用层补录路径）")

	require.ElementsMatch(t, []string{"gpt-image-2", "gpt-4o", "dall-e-3"},
		collectPlaygroundModelIDs(apps[PlaygroundAppImage].Groups),
		"空 kind 的图片名模型不得漏出生图台")

	require.ElementsMatch(t, []string{"gpt-image-2", "gpt-4o", "dall-e-3"},
		collectPlaygroundModelIDs(apps[PlaygroundAppCanvas].Groups))
}
