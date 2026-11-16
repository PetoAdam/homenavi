import { getDisplayPresentationOptions, isValidControlOverride, sanitizeInputKey } from '../components/common/DeviceControlRenderer/deviceControlUtils';

export function inputIdentityKeys(input) {
  const keys = new Set();
  [input?.property, input?.capability_id, input?.capabilityId, input?.capabilityID, input?.id].forEach(value => {
    const key = String(value || '').trim().toLowerCase();
    if (!key) return;
    keys.add(key);
    if (key.startsWith('set_')) keys.add(key.slice(4));
  });
  return [...keys];
}

export function applyInputPresentation(input, meta) {
  const control = meta?.presentation?.controls?.[sanitizeInputKey(input)];
  if (typeof control !== 'string') return input;
  const candidate = { ...input, ui: { ...input.ui, control } };
  return isValidControlOverride(candidate) ? candidate : input;
}

export function applyDevicePresentation(device) {
  const meta = device?.meta;
  const applyCapability = capability => {
    const key = capability?.id || capability?.property || capability?.name;
    const display = meta?.presentation?.displays?.[key];
    if (typeof display !== 'string' || !getDisplayPresentationOptions(capability).includes(display)) return capability;
    return { ...capability, ui: { ...capability.ui, display } };
  };
  const capabilities = Array.isArray(device?.capabilities) ? device.capabilities.map(applyCapability) : device?.capabilities;
  const stateCapabilities = Array.isArray(device?.state?.capabilities) ? device.state.capabilities.map(applyCapability) : device?.state?.capabilities;
  const allCapabilities = [...(Array.isArray(capabilities) ? capabilities : []), ...(Array.isArray(stateCapabilities) ? stateCapabilities : [])];
  const inputs = Array.isArray(device?.inputs) ? device.inputs.map(input => {
    const explicitID = input.capability_id || input.capabilityId || input.capabilityID;
    const keys = [input.id, input.property]
      .filter(Boolean).map(key => String(key).toLowerCase());
    const capability = explicitID
      ? allCapabilities.find(cap => String(cap?.id || '').toLowerCase() === String(explicitID).toLowerCase())
      : keys.map(key => allCapabilities.find(cap => String(cap?.id || '').toLowerCase() === key)
      || allCapabilities.find(cap => [cap?.property, cap?.name].some(value => value && String(value).toLowerCase() === key)))
        .find(Boolean);
    if (explicitID && !capability) return input;
    const candidate = applyInputPresentation({ ...input, capability, range: input.range || input.Range || null }, meta);
    if (candidate.ui === input.ui) return input;
    return { ...input, ui: candidate.ui };
  }) : device?.inputs;
  return {
    ...device,
    ...(Array.isArray(device?.inputs) ? { inputs } : {}),
    ...(Array.isArray(device?.capabilities) ? { capabilities } : {}),
    ...(Array.isArray(device?.state?.capabilities) ? { state: { ...device.state, capabilities: stateCapabilities } } : {}),
  };
}