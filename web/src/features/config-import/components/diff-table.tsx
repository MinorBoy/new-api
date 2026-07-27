/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { ReactNode } from 'react'

export interface DiffTableColumn<T> {
  id: string
  header: ReactNode
  cell: (row: T) => ReactNode
}

export interface DiffTableProps<T> {
  columns: DiffTableColumn<T>[]
  rows: T[]
  getRowKey: (row: T, index: number) => string
  emptyMessage: ReactNode
}

export function DiffTable<T>(props: DiffTableProps<T>) {
  return (
    <div className='overflow-x-auto border'>
      <table className='w-full min-w-[58rem] text-left text-sm'>
        <thead className='bg-muted/50 text-muted-foreground'>
          <tr>
            {props.columns.map((column) => (
              <th key={column.id} className='px-3 py-2 font-medium'>
                {column.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {props.rows.length === 0 ? (
            <tr className='border-t'>
              <td
                className='text-muted-foreground px-3 py-5 text-center'
                colSpan={props.columns.length}
              >
                {props.emptyMessage}
              </td>
            </tr>
          ) : (
            props.rows.map((row, index) => (
              <tr key={props.getRowKey(row, index)} className='border-t'>
                {props.columns.map((column) => (
                  <td key={column.id} className='px-3 py-2 align-top'>
                    {column.cell(row)}
                  </td>
                ))}
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  )
}
