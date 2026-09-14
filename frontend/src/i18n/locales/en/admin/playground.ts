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
      'When set, workbenches inject the model by its marked kind (chat / image / video / audio) instead of guessing from the model name',
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
