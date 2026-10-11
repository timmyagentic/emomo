export type AccountAuthority = { authenticate(token: string): Promise<boolean> };
export type AccountNamespace = { getByName(name: string): AccountAuthority };
