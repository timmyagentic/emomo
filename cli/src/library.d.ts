export interface LibraryRecord {
  id: string; sourceId: string; description: string; imageText: string; category: 'usable' | 'object_sticker';
  tags: string[]; subjects: string[]; scenarios: string[]; aliases: string[]; contentFlags: string[];
  quality: string; mediaKind: 'image' | 'preview' | 'animation'; previewOnly: boolean;
  image: { width: number; height: number; frames: number; format: 'png' | 'jpeg' | 'webp' | 'gif' };
  sha256: string; publicReleaseClearance: string; asset: string;
}
export interface LibrarySnapshot {
  root: string;
  manifest: { schemaVersion: number; format: string; collection: string; revision: string; contentRevision: string; records: number };
  rows: LibraryRecord[];
  vocabulary: { intents: Record<string,string[]>; subjects: Record<string,string[]>; gaps: Record<string,string> };
  report: Record<string,unknown>;
}
export function validateSnapshot(directory: string): Promise<LibrarySnapshot>;
