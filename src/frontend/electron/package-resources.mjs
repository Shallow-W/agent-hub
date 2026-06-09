import { copyFileSync, existsSync, mkdirSync, rmSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export function repoRootFromElectronDir(electronDir = __dirname) {
  return path.resolve(electronDir, '..', '..', '..');
}

export function backendBinaryName(platform = process.platform) {
  return platform === 'win32' ? 'server.exe' : 'server';
}

export function chooseConfigSource({ repoRoot, exists = existsSync }) {
  const localConfig = path.join(repoRoot, 'src', 'backend', 'config', 'config.yaml');
  if (exists(localConfig)) {
    return localConfig;
  }
  return path.join(repoRoot, 'src', 'backend', 'config', 'config.example.yaml');
}

export function prepareResourcePlan({
  repoRoot = repoRootFromElectronDir(),
  platform = process.platform,
  exists = existsSync,
} = {}) {
  const resourcesDir = path.join(repoRoot, 'desktop-resources');
  return {
    repoRoot,
    backendDir: path.join(repoRoot, 'src', 'backend'),
    backendOutput: path.join(resourcesDir, 'bin', backendBinaryName(platform)),
    configSource: chooseConfigSource({ repoRoot, exists }),
    configOutput: path.join(resourcesDir, 'config', 'config.yaml'),
    resourcesDir,
  };
}

function run(command, args, options) {
  const result = spawnSync(command, args, {
    stdio: 'inherit',
    ...options,
  });
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(' ')} failed with exit code ${result.status}`);
  }
}

export function prepareDesktopResources(options = {}) {
  const plan = prepareResourcePlan(options);
  rmSync(plan.resourcesDir, { recursive: true, force: true });
  mkdirSync(path.dirname(plan.backendOutput), { recursive: true });
  mkdirSync(path.dirname(plan.configOutput), { recursive: true });

  run('go', ['build', '-o', plan.backendOutput, './cmd/server'], { cwd: plan.backendDir });
  copyFileSync(plan.configSource, plan.configOutput);
  return plan;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  prepareDesktopResources();
}
