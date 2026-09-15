import { describe, expect, it } from "vitest";

import {
  buildInjectedFingerprint,
  filterModelsByAllowedKinds,
  type PlaygroundInjectedConfig
} from "../playground";

/** 构造最小可用的注入配置（单分组单模型） */
function makeInjectedConfig(overrides?: Partial<PlaygroundInjectedConfig>): PlaygroundInjectedConfig {
  return {
    app: "chat",
    apiUrl: "https://api.example.com/v1",
    apiKey: "sk-test",
    groupName: "default",
    models: ["gpt-4o"],
    groups: [
      {
        groupName: "default",
        apiUrl: "https://api.example.com/v1",
        apiKey: "sk-test",
        models: [
          {
            model_id: "gpt-4o",
            display_name: "GPT-4o 🟢 可用",
            price_label: "$1",
            unit_hint: "1K tokens",
            description: "",
            model_kind: "chat",
            monitor_status: "operational"
          }
        ]
      }
    ],
    ...overrides
  };
}

describe("buildInjectedFingerprint", () => {
  it("returns empty string for null config", () => {
    expect(buildInjectedFingerprint(null)).toBe("");
  });

  it("is stable for identical content", () => {
    expect(buildInjectedFingerprint(makeInjectedConfig())).toBe(
      buildInjectedFingerprint(makeInjectedConfig())
    );
  });

  // 核心场景：渠道监控变红后指纹必须变化，周期刷新才能发现并重注入
  it("changes when monitor_status degrades", () => {
    const healthy = buildInjectedFingerprint(makeInjectedConfig());
    const failed = buildInjectedFingerprint(
      makeInjectedConfig({
        groups: [
          {
            groupName: "default",
            apiUrl: "https://api.example.com/v1",
            apiKey: "sk-test",
            models: [
              {
                model_id: "gpt-4o",
                display_name: "GPT-4o 🔴 异常",
                price_label: "$1",
                unit_hint: "1K tokens",
                description: "",
                model_kind: "chat",
                monitor_status: "failed"
              }
            ]
          }
        ]
      })
    );
    expect(failed).not.toBe(healthy);
  });

  it("changes when display_name / apiKey / top-level models change", () => {
    const base = buildInjectedFingerprint(makeInjectedConfig());
    expect(
      buildInjectedFingerprint(
        makeInjectedConfig({
          groups: [
            {
              groupName: "default",
              apiUrl: "https://api.example.com/v1",
              apiKey: "sk-test",
              models: [
                {
                  model_id: "gpt-4o",
                  display_name: "GPT-4o（改名）🟢 可用",
                  price_label: "$1",
                  unit_hint: "1K tokens",
                  description: "",
                  model_kind: "chat",
                  monitor_status: "operational"
                }
              ]
            }
          ]
        })
      )
    ).not.toBe(base);
    expect(buildInjectedFingerprint(makeInjectedConfig({ apiKey: "sk-rotated" }))).not.toBe(base);
    expect(
      buildInjectedFingerprint(makeInjectedConfig({ models: ["gpt-4o", "gpt-4o-mini"] }))
    ).not.toBe(base);
  });
});

describe("filterModelsByAllowedKinds", () => {
  const items = [
    { model_id: "gpt-4o", model_kind: "chat" },
    { model_id: "gpt-image-1", model_kind: "image" },
    { model_id: "sora-2", model_kind: "video" },
    { model_id: "tts-1", model_kind: "audio" }
  ];

  it("chat app keeps only chat models", () => {
    expect(filterModelsByAllowedKinds("chat", items).map((m) => m.model_id)).toEqual(["gpt-4o"]);
  });

  it("image / canvas apps keep chat + image models in order", () => {
    const expected = ["gpt-4o", "gpt-image-1"];
    expect(filterModelsByAllowedKinds("image", items).map((m) => m.model_id)).toEqual(expected);
    expect(filterModelsByAllowedKinds("canvas", items).map((m) => m.model_id)).toEqual(expected);
  });

  // 与后端口径一致：空 kind 的存量模型先按模型名推断再过滤
  it("infers kind from model name when model_kind is empty", () => {
    const legacy = [
      { model_id: "gpt-image-2", model_kind: "" },
      { model_id: "gpt-4o", model_kind: "" }
    ];
    // 空 kind 的图片模型不得混进对话台
    expect(filterModelsByAllowedKinds("chat", legacy).map((m) => m.model_id)).toEqual(["gpt-4o"]);
    // 也不得漏出生图台
    expect(filterModelsByAllowedKinds("image", legacy).map((m) => m.model_id)).toEqual([
      "gpt-image-2",
      "gpt-4o"
    ]);
  });

  it("prefers explicit model_kind over name inference", () => {
    // model_id 含 image 关键词但显式标记为 chat：显式 kind 优先
    const explicit = [{ model_id: "image-understanding-chat", model_kind: "chat" }];
    expect(filterModelsByAllowedKinds("chat", explicit)).toHaveLength(1);
  });
});
