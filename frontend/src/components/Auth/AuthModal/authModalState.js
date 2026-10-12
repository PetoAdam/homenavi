export function requiresTwoFactor(result) {
  return Boolean(result?.twoFA);
}