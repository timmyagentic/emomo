export const MAX_IMPORT_IMAGE_BYTES: number;
export const SHA256: RegExp;
export type PreparedMetadata = { sha256: string; image_info: {width:number; height:number; format:number}; category:string; description:string; ocr_text:string; text_presence:number; tags:string[]; search_aliases:string[]; preview_only:boolean };
export function importMetadata(value: unknown): PreparedMetadata;
