function quoteArg(value: string): string {
  return `"${value.replace(/"/g, '\\"')}"`;
}

function readCommandArg(command: string, name: string): string {
  const escaped = name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const match = command.match(new RegExp(`--${escaped}\\s+(?:"([^"]+)"|(\\S+))`));
  return match?.[1] ?? match?.[2] ?? '';
}

export const DAEMON_PACKAGE_SPEC = '@hust-agenthub/daemon@0.3.0';
export const DAEMON_VERSION = '0.3.0';

export function buildCommands(
  backendCommand: string,
  daemonNPMPath: string,
  machineName: string,
): { npx: string; node: string } {
  const serverURL = readCommandArg(backendCommand, 'server-url');
  const apiKey = readCommandArg(backendCommand, 'api-key');
  const npx = `npx ${quoteArg(DAEMON_PACKAGE_SPEC)} --server-url ${quoteArg(serverURL)} --api-key ${quoteArg(apiKey)} # ${machineName}`;
  const trimmedPath = (daemonNPMPath ?? '').replace(/\/$/, '');
  const entry = `${trimmedPath}/bin/agenthub-daemon.js`;
  const node = `node ${quoteArg(entry)} --server-url ${quoteArg(serverURL)} --api-key ${quoteArg(apiKey)} # ${machineName}`;
  return { npx, node };
}
