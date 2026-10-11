export type Operation = 'search' | 'read' | 'download';
export type Quota = { hour: number; day: number };
export const DEFAULT_QUOTAS: Record<Operation, Quota> = {
  search: { hour: 120, day: 500 }, read: { hour: 600, day: 2000 }, download: { hour: 120, day: 500 },
};
