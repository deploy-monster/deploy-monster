import { describe, it, expect } from 'vitest';
import {
  api,
  appsAPI,
  domainsAPI,
  serversAPI,
  databasesAPI,
  backupsAPI,
  gitSourcesAPI,
  teamAPI,
  secretsAPI,
  billingAPI,
  monitoringAPI,
  marketplaceAPI,
  adminAPI,
  dashboardAPI,
  authAPI,
} from '../index';

describe('API barrel exports', () => {
  it('exports the base api client', () => {
    expect(api).toBeDefined();
    expect(typeof api.get).toBe('function');
    expect(typeof api.post).toBe('function');
    expect(typeof api.put).toBe('function');
    expect(typeof api.patch).toBe('function');
    expect(typeof api.delete).toBe('function');
  });

  const modules = [
    { name: 'appsAPI', mod: appsAPI, methods: ['list', 'get', 'create', 'delete', 'start', 'stop', 'restart'] },
    { name: 'domainsAPI', mod: domainsAPI, methods: ['list', 'create', 'verify', 'delete'] },
    { name: 'serversAPI', mod: serversAPI, methods: ['list', 'create'] },
    { name: 'databasesAPI', mod: databasesAPI, methods: ['list', 'create'] },
    { name: 'backupsAPI', mod: backupsAPI, methods: ['list', 'create', 'restore'] },
    { name: 'gitSourcesAPI', mod: gitSourcesAPI, methods: ['list', 'connect', 'disconnect'] },
    { name: 'teamAPI', mod: teamAPI, methods: ['members', 'invite', 'removeMember', 'auditLog'] },
    { name: 'secretsAPI', mod: secretsAPI, methods: ['list', 'create', 'delete'] },
    { name: 'billingAPI', mod: billingAPI, methods: ['plans', 'usage'] },
    { name: 'monitoringAPI', mod: monitoringAPI, methods: ['serverMetrics', 'alerts'] },
    { name: 'marketplaceAPI', mod: marketplaceAPI, methods: ['list', 'deploy'] },
    { name: 'adminAPI', mod: adminAPI, methods: ['system', 'tenants', 'saveSettings', 'generateApiKey'] },
    { name: 'dashboardAPI', mod: dashboardAPI, methods: ['stats', 'activity', 'announcements', 'search'] },
    { name: 'authAPI', mod: authAPI, methods: ['login', 'register', 'refresh'] },
  ];

  for (const { name, mod, methods } of modules) {
    it(`exports ${name} with expected methods`, () => {
      expect(mod).toBeDefined();
      for (const method of methods) {
        expect(typeof (mod as Record<string, unknown>)[method]).toBe('function');
      }
    });
  }
});
