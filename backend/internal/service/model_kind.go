package service

import "strings"

// InferModelKind 按模型名关键词推断模型类型（模型类型分流的唯一事实源）。
//
// 规则：大小写不敏感，关键词匹配模型名任意位置；按 image → video → audio 的
// 顺序命中即返回，全部未命中返回 chat。
//
// fork: 三层防线（保存时自动填充 normalizeModels / 下发时兜底 filterModelsByAllowedKinds /
// 前端兜底展示 inferModelKind）都收敛到本表。
// 前端兜底推断（frontend/src/config/playground.ts 的 inferModelKind）需与本表保持一致，
// 修改任一侧时必须同步另一侧。
func InferModelKind(modelID string) string {
	name := strings.ToLower(strings.TrimSpace(modelID))
	if name == "" {
		return ModelKindChat
	}
	for _, group := range modelKindKeywordTable {
		for _, keyword := range group.keywords {
			if strings.Contains(name, keyword) {
				return group.kind
			}
		}
	}
	return ModelKindChat
}

// modelKindKeywordTable 模型类型关键词表（顺序即匹配优先级：image → video → audio）。
var modelKindKeywordTable = []struct {
	kind     string
	keywords []string
}{
	{
		kind: ModelKindImage,
		keywords: []string{
			"image", "gpt-image", "dall-e", "dall_e", "flux",
			"stable-diffusion", "sdxl", "midjourney", "mj-",
		},
	},
	{
		kind: ModelKindVideo,
		keywords: []string{
			"sora", "veo", "kling", "runway", "wanx", "hunyuan-video", "seedance",
		},
	},
	{
		kind: ModelKindAudio,
		keywords: []string{
			"tts", "whisper", "speech", "audio", "voice", "suno",
		},
	},
}
