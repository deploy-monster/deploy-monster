import { describe, expect, it } from 'vitest';
import { formatGeneratedSecrets, generatedSecretEntries } from '../generatedSecrets';

describe('generated secret formatting', () => {
  it('sorts generated credentials by key', () => {
    expect(generatedSecretEntries({
      REDIS_PASSWORD: 'redis-secret',
      DB_PASSWORD: 'db-secret',
    })).toEqual([
      ['DB_PASSWORD', 'db-secret'],
      ['REDIS_PASSWORD', 'redis-secret'],
    ]);
  });

  it('formats generated credentials as env assignments', () => {
    expect(formatGeneratedSecrets({
      REDIS_PASSWORD: 'redis-secret',
      DB_PASSWORD: 'db-secret',
    })).toBe('DB_PASSWORD=db-secret\nREDIS_PASSWORD=redis-secret');
  });
});
