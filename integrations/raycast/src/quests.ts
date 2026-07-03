import { execFile } from "child_process";
import { existsSync } from "fs";
import { homedir } from "os";
import { promisify } from "util";

const run = promisify(execFile);

/** Resolve the quests binary, preferring the `make install` build (which has
 * the `add`/`campaigns` subcommands) over whatever is on PATH. */
export function questsBin(): string {
  const candidates = [`${homedir()}/go/bin/quests`, "/opt/homebrew/bin/quests", "/usr/local/bin/quests"];
  return candidates.find(existsSync) ?? "quests";
}

/** Run `quests add` with the given flags/args. */
export async function questsAdd(args: string[]): Promise<void> {
  await run(questsBin(), ["add", ...args]);
}

/** Active campaign names, or [] if none / the binary can't be reached. */
export async function listCampaigns(): Promise<string[]> {
  try {
    const { stdout } = await run(questsBin(), ["campaigns"]);
    return stdout
      .split("\n")
      .map((s) => s.trim())
      .filter(Boolean);
  } catch {
    return [];
  }
}
