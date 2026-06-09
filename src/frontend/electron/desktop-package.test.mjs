import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import {
  chooseConfigSource,
  prepareResourcePlan,
} from './package-resources.mjs';

describe('desktop package resource preparation', () => {
  it('uses the real backend config when it exists', () => {
    const repoRoot = path.join('D:', 'Agent', 'agent-hub');
    const realConfig = path.join(repoRoot, 'src', 'backend', 'config', 'config.yaml');

    assert.equal(
      chooseConfigSource({
        repoRoot,
        exists: (candidate) => candidate === realConfig,
      }),
      realConfig,
    );
  });

  it('falls back to config.example.yaml when no local config exists', () => {
    const repoRoot = path.join('D:', 'Agent', 'agent-hub');

    assert.equal(
      chooseConfigSource({
        repoRoot,
        exists: () => false,
      }),
      path.join(repoRoot, 'src', 'backend', 'config', 'config.example.yaml'),
    );
  });

  it('plans backend binary and config outputs under the desktop resources directory', () => {
    const repoRoot = path.join('D:', 'Agent', 'agent-hub');
    const plan = prepareResourcePlan({
      repoRoot,
      platform: 'win32',
      exists: () => true,
    });

    assert.equal(plan.backendOutput, path.join(repoRoot, 'desktop-resources', 'bin', 'server.exe'));
    assert.equal(plan.configOutput, path.join(repoRoot, 'desktop-resources', 'config', 'config.yaml'));
  });
});
