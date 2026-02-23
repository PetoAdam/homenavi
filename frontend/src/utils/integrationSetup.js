export function hasSetupUiPath(integration) {
  return typeof integration?.setup_ui_path === 'string' && integration.setup_ui_path.trim().length > 0;
}

export function setupRouteForIntegration(integration) {
  if (!integration?.id || !hasSetupUiPath(integration)) return '';
  return `/apps/${integration.id}/setup`;
}
