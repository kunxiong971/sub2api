export default {
  playground: {
    title: 'Workbench Config',
    description:
      'Configure injection rules for Chat / Image / Canvas: bind a group and the model list to display (no credentials stored here)',
    tabs: {
      chat: 'Chat',
      image: 'Image',
      canvas: 'Canvas',
      global: 'Global config'
    },
    globalHint:
      'Maintain channels + model lists once here: add a channel, fetch its models, set display name / price / kind. Chat injects "Chat (LLM)" models; Image and Canvas inject "Chat (LLM) + Image". App tabs remain only as per-app overrides.',
    globalTag: 'Global',
    globalRowHint: 'Effective model from the global config — edit it on the Global config tab',
    basic: 'Basic',
    addGroup: 'Add group',
    removeGroup: 'Remove',
    bindingCount: '{count} group(s) bound',
    noBindings: 'No groups bound yet',
    noBindingsHint: 'Pick a group from the dropdown above and click "Add group" to add a channel',
    models: 'Models',
    enabled: 'Enable this group',
    enabledHint: 'When disabled, customers no longer receive injection config for this workbench',
    group: 'Bound group',
    groupPlaceholder: 'Select a group',
    groupHint:
      'At runtime the customer\'s own key in this group is injected; no key is stored in this config',
    save: 'Save',
    saving: 'Saving…',
    saved: 'Saved',
    fetchModels: 'Fetch models',
    fetching: 'Fetching…',
    fetchHint: 'Pull available models from the selected group to avoid typos',
    searchModel: 'Search models…',
    col: {
      model: 'Model',
      displayName: 'Display name',
      priceLabel: 'Price label',
      unitHint: 'Unit hint',
      description: 'Description',
      kind: 'Kind',
      monitor: 'Channel monitor',
      sortOrder: 'Order',
      enabled: 'Enabled',
      actions: 'Actions'
    },
    monitorNone: 'None',
    kindAuto: 'Auto',
    kindChat: 'Chat (LLM)',
    kindImage: 'Image',
    kindVideo: 'Video',
    kindAudio: 'Audio',
    kindHint:
      'When set, workbenches inject the model by its marked kind; leaving it empty auto-fills the kind from the model name on save',
    kindInferredAs: 'Auto: {kind}',
    kindInferredHint:
      'When unset, the kind is auto-detected from the model name on save (same rules as injection routing; the backend is the single source of truth). An explicit kind always takes precedence',
    legacyBanner:
      'This tab shows legacy per-app entries (read-only). Use "Move to global" to merge models into the global config (same-group merge with dedup); unmigrated entries keep working as before.',
    migrateToGlobal: 'Move to global',
    migrating: 'Moving…',
    migrated: 'Moved to the global config',
    migratedNothing: 'This binding had no models of its own (models come from the global config); the legacy entry was cleaned up',
    selfCheck: 'Self-check',
    selfCheckLoading: 'Checking…',
    selfCheckTitle: 'Injection self-check — what each workbench actually receives',
    selfCheckHint:
      'The lists below are what the three workbenches currently receive (kind-routed, with monitor status and long-context thresholds; no secrets included).',
    selfCheckClose: 'Close',
    selfCheckEmpty: 'No models are injected into this workbench',
    selfCheckModelCount: '{count} models',
    selfCheckLongCtx: 'long context ≥{threshold}',
    monitorHint:
      'When linked, the model shows a channel status tag (available / down) in Chat / Image / Canvas',
    priceHint:
      'Display name and price label are display-only and never affect billing; billing still follows group multipliers and pricing rules',
    selectAll: 'Select all',
    selectedCount: '{count} models selected',
    emptyModels: 'No models configured yet',
    emptyHint: 'Click "Fetch models" to load the models available in this group, then select the ones to expose',
    errors: {
      loadFailed: 'Failed to load config',
      saveFailed: 'Failed to save config',
      fetchFailed: 'Failed to fetch models',
      modelsSaveFailed: 'Failed to save model list',
      groupRequired: 'Please select a group first'
    }
  }
}
