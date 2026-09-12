export const configImportQueryKeys = {
  all: ['config-import'] as const,
  batches: () => [...configImportQueryKeys.all, 'batches'] as const,
  list: (params: { page?: number; page_size?: number } = {}) =>
    [...configImportQueryKeys.batches(), 'list', params] as const,
  detail: (id: number) => [...configImportQueryKeys.batches(), id] as const,
} as const
