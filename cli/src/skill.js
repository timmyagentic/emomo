import { homedir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { lstat, mkdir, readFile, rename, rm, mkdtemp, cp } from 'node:fs/promises';
import { EmomoError } from './error.js';

export const SKILL_SOURCE = fileURLToPath(new URL('../skills/emomo/', import.meta.url));
const FILES = ['SKILL.md', 'agents/openai.yaml'];

export async function installSkill({ agent = 'codex', directory, env = process.env } = {}) {
  const homes = { codex: join(env.CODEX_HOME || join(homedir(), '.codex'), 'skills'), claude: join(homedir(), '.claude', 'skills'), agents: join(homedir(), '.agents', 'skills') };
  if (!Object.hasOwn(homes, agent)) throw new EmomoError('INVALID_ARGUMENT', 'Supported agents: codex, claude, agents.');
  const parent = resolve(directory || homes[agent]);
  const destination = join(parent, 'emomo');
  try {
    const stat = await lstat(destination);
    if (!stat.isDirectory() || stat.isSymbolicLink()) throw new EmomoError('SKILL_EXISTS', 'The skill destination already exists. Use a different skills directory.');
    const same = await Promise.all(FILES.map(async file => (await readFile(join(destination, file))).equals(await readFile(join(SKILL_SOURCE, file)))));
    if (same.every(Boolean)) return { agent, path: destination, alreadyInstalled: true };
    throw new EmomoError('SKILL_EXISTS', 'A different emomo skill already exists. Review it before replacing it.');
  } catch (error) {
    if (error instanceof EmomoError) throw error;
    // An incomplete existing skill must not be replaced either.
    if (error.code !== 'ENOENT') throw new EmomoError('SKILL_INSTALL_FAILED', 'The skill directory could not be inspected.');
    try {
      await lstat(destination);
      throw new EmomoError('SKILL_EXISTS', 'An incomplete emomo skill already exists. Review it before replacing it.');
    } catch (check) { if (check.code !== 'ENOENT') throw check; }
  }
  await mkdir(parent, { recursive: true });
  const temporary = await mkdtemp(join(parent, '.emomo-skill-'));
  let ownsDestination = false;
  try {
    await cp(SKILL_SOURCE, temporary, { recursive: true });
    // Reserve the name exclusively so an existing empty user directory is safe too.
    await mkdir(destination);
    ownsDestination = true;
    await rename(temporary, destination);
  } catch {
    if (ownsDestination) await rm(destination, { recursive: true, force: true });
    throw new EmomoError('SKILL_INSTALL_FAILED', 'The skill could not be installed. Check directory permissions and existing files.');
  } finally {
    await rm(temporary, { recursive: true, force: true });
  }
  return { agent, path: destination, alreadyInstalled: false };
}
