/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
export const configImportQueryKeys = {
  all: ['config-import'] as const,
  batches: () => [...configImportQueryKeys.all, 'batches'] as const,
  list: (params: { page?: number; page_size?: number } = {}) =>
    [...configImportQueryKeys.batches(), 'list', params] as const,
  detail: (id: number) => [...configImportQueryKeys.batches(), id] as const,
} as const
