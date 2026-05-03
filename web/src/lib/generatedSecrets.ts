export function generatedSecretEntries(secrets: Record<string, string>) {
  return Object.entries(secrets).sort(([a], [b]) => a.localeCompare(b));
}

export function formatGeneratedSecrets(secrets: Record<string, string>) {
  return generatedSecretEntries(secrets)
    .map(([key, value]) => `${key}=${value}`)
    .join('\n');
}
